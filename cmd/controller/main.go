/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"net/http"
	"net/url"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3iocontroller "github.com/metal3-io/baremetal-operator/controllers/metal3.io"
	"github.com/metal3-io/baremetal-operator/pkg/secretutils"
	"github.com/openshift/image-customization-controller/pkg/env"
	"github.com/openshift/image-customization-controller/pkg/imagehandler"
	"github.com/openshift/image-customization-controller/pkg/imageprovider"
	"github.com/openshift/image-customization-controller/pkg/version"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = k8sruntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	infraEnvLabel string = "infraenvs.agent-install.openshift.io"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3iov1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func setupChecks(mgr ctrl.Manager) error {
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		return err
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		return err
	}
	return nil
}

func runController(watchNamespace string, imageServer imagehandler.ImageHandler, envInputs *env.EnvInputs) error {
	excludeInfraEnv, err := labels.NewRequirement(infraEnvLabel, selection.DoesNotExist, nil)
	if err != nil {
		setupLog.Error(err, "cannot create an infraenv label filter")
		return err
	}

	cacheOptions := cache.Options{
		SelectorsByObject: secretutils.AddSecretSelector(cache.SelectorsByObject{
			&metal3iov1alpha1.PreprovisioningImage{}: cache.ObjectSelector{
				Label: labels.NewSelector().Add(*excludeInfraEnv),
			},
		}),
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                scheme,
		Port:                  0, // Add flag with default of 9443 when adding webhooks
		Namespace:             watchNamespace,
		ClientDisableCacheFor: []client.Object{&corev1.Secret{}},
		NewCache:              cache.BuilderWithOptions(cacheOptions),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	imgReconciler := metal3iocontroller.PreprovisioningImageReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PreprovisioningImage"),
		APIReader:     mgr.GetAPIReader(),
		Scheme:        mgr.GetScheme(),
		ImageProvider: imageprovider.NewRHCOSImageProvider(imageServer, envInputs),
	}
	if err = (&imgReconciler).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PreprovisioningImage")
		return err
	}

	// +kubebuilder:scaffold:builder

	if err := setupChecks(mgr); err != nil {
		return err
	}

	setupLog.Info("starting manager")
	return mgr.Start(ctrl.SetupSignalHandler())
}

func main() {
	var watchNamespace string
	var devLogging bool
	var imagesBindAddr string
	var imagesPublishAddr string

	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	flag.StringVar(&watchNamespace, "namespace", os.Getenv("WATCH_NAMESPACE"),
		"Namespace that the controller watches to reconcile preprovisioningimage resources.")
	flag.StringVar(&imagesBindAddr, "images-bind-addr", ":8084",
		"The address the images endpoint binds to.")
	flag.StringVar(&imagesPublishAddr, "images-publish-addr", "http://127.0.0.1:8084",
		"The address clients would access the images endpoint from.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(devLogging)))

	version.Print(setupLog)

	envInputs, err := env.New()
	if err != nil {
		setupLog.Error(err, "environment not provided")
		os.Exit(1)
	}

	publishURL, err := url.Parse(imagesPublishAddr)
	if err != nil {
		setupLog.Error(err, "imagesPublishAddr is not parsable")
		os.Exit(1)
	}

	imageServer := imagehandler.NewImageHandler(ctrl.Log.WithName("ImageHandler"), envInputs.DeployISO, envInputs.DeployInitrd, publishURL)
	http.Handle("/", http.FileServer(imageServer.FileSystem()))

	go func() {
		server := &http.Server{
			Addr:              imagesBindAddr,
			ReadHeaderTimeout: 5 * time.Second,
		}

		err := server.ListenAndServe()

		if err != nil {
			setupLog.Error(err, "")
			os.Exit(1)
		}
	}()

	if err := runController(watchNamespace, imageServer, envInputs); err != nil {
		setupLog.Error(err, "problem running controller")
		os.Exit(1)
	}
}
