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
	"context"
	"flag"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metal3iov1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	metal3iocontroller "github.com/metal3-io/baremetal-operator/controllers/metal3.io"
	"github.com/openshift/image-customization-controller/pkg/env"
	"github.com/openshift/image-customization-controller/pkg/ignition"
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
	infraEnvLabel                string = "infraenvs.agent-install.openshift.io"
	ignitionSecretName           string = "metal3-ironic-agent-config"
	ignitionSecretAnnotationName        = "baremetal.openshift.io/metal3-agent-config"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metal3iov1alpha1.AddToScheme(scheme)

	_ = corev1.AddToScheme(scheme)
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

type PreprovisioningImageReconciler struct {
	metal3iocontroller.PreprovisioningImageReconciler
	envInputs *env.EnvInputs
}

func (r *PreprovisioningImageReconciler) ensureIgnitionSecret(ctx context.Context, log logr.Logger, req ctrl.Request, img *metal3iov1alpha1.PreprovisioningImage) (ctrl.Result, error) {
	ignitionBuilder, err := ignition.Base(
		r.envInputs.IronicBaseURL, r.envInputs.IronicInspectorBaseURL, r.envInputs.IronicAgentImage,
		r.envInputs.HttpsProxy, r.envInputs.HttpsProxy, r.envInputs.NoProxy)
	if err != nil {
		return ctrl.Result{}, err
	}

	name := types.NamespacedName{Name: ignitionSecretName, Namespace: req.Namespace}
	sublog := log.WithValues("ignitionSecret", name)

	secret := &corev1.Secret{}
	err = r.Get(ctx, name, secret)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			sublog.Error(err, "unexpected error when looking for an ignition secret")
			return ctrl.Result{Requeue: true}, err
		}

		sublog.Info("creating secret")

		ignitionData, err := ignitionBuilder.Generate()
		if err != nil {
			sublog.Error(err, "cannot generate ignition secret")
			return ctrl.Result{}, err
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ignitionSecretName,
				Namespace: req.Namespace,
			},
			Data: map[string][]byte{
				"userData": ignitionData,
			},
		}

		err = r.Create(ctx, secret)
		if err != nil {
			sublog.Error(err, "cannot create ignition secret, will retry")
			// Quite likely transient, possibly a race
			return ctrl.Result{Requeue: true}, err
		}
	}

	patch := client.MergeFrom(img.DeepCopy())
	if img.Annotations == nil {
		img.Annotations = make(map[string]string, 1)
	}
	img.Annotations[ignitionSecretAnnotationName] = ignitionSecretName

	sublog.Info("linking the ignition secret in the annotation")
	err = r.PreprovisioningImageReconciler.Client.Patch(ctx, img, patch)
	// return ctrl.Result{Requeue: err != nil}, err
	return ctrl.Result{Requeue: true}, err
}

func (r *PreprovisioningImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("preprovisioningimage", req.NamespacedName)

	img := &metal3iov1alpha1.PreprovisioningImage{}
	err := r.Get(ctx, req.NamespacedName, img)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("PreprovisioningImage not found")
			err = nil
		}
		return ctrl.Result{}, err
	}

	// if img.Labels[infraEnvLabel] != "" {
	_ = infraEnvLabel
	if img.DeletionTimestamp.IsZero() && img.Annotations[ignitionSecretAnnotationName] == "" {
		return r.ensureIgnitionSecret(ctx, log, req, img)
	}
	// Don't handle images with an InfraEnv label beyond the annotation.
	// return ctrl.Result{}, nil
	// }

	return r.PreprovisioningImageReconciler.Reconcile(ctx, req)
}

func (r *PreprovisioningImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metal3iov1alpha1.PreprovisioningImage{}).
		Owns(&corev1.Secret{}, builder.MatchEveryOwner).
		Complete(r)
}

func runController(watchNamespace string, imageServer imagehandler.ImageHandler, envInputs *env.EnvInputs, metricsBindAddr string) error {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		Port:               0, // Add flag with default of 9443 when adding webhooks
		Namespace:          watchNamespace,
		MetricsBindAddress: metricsBindAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	imgReconciler := PreprovisioningImageReconciler{
		PreprovisioningImageReconciler: metal3iocontroller.PreprovisioningImageReconciler{
			Client:        mgr.GetClient(),
			Log:           ctrl.Log.WithName("controllers").WithName("PreprovisioningImage"),
			APIReader:     mgr.GetAPIReader(),
			Scheme:        mgr.GetScheme(),
			ImageProvider: imageprovider.NewRHCOSImageProvider(imageServer, envInputs),
		},
		envInputs: envInputs,
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
	var metricsBindAddr string
	var devLogging bool
	var imagesBindAddr string
	var imagesPublishAddr string

	// From CAPI point of view, BMO should be able to watch all namespaces
	// in case of a deployment that is not multi-tenant. If the deployment
	// is for multi-tenancy, then the BMO should watch only the provided
	// namespace.
	flag.StringVar(&watchNamespace, "namespace", os.Getenv("WATCH_NAMESPACE"),
		"Namespace that the controller watches to reconcile preprovisioningimage resources.")
	flag.StringVar(&metricsBindAddr, "metrics-addr", "",
		"The address the metric endpoint binds to.")
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

	// If not defined via env var, look for the mounted secret file
	if envInputs.IronicAgentPullSecret == "" {
		pullSecretRaw, err := os.ReadFile("/run/secrets/pull-secret")
		if err != nil {
			setupLog.Error(err, "unable to read secret from mounted file")
			os.Exit(1)
		}
		envInputs.IronicAgentPullSecret = string(pullSecretRaw)
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

	if err := runController(watchNamespace, imageServer, envInputs, metricsBindAddr); err != nil {
		setupLog.Error(err, "problem running controller")
		os.Exit(1)
	}
}
