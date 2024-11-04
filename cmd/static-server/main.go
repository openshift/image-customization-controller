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
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/image-customization-controller/pkg/env"
	"github.com/openshift/image-customization-controller/pkg/ignition"
	"github.com/openshift/image-customization-controller/pkg/imagehandler"
	"github.com/openshift/image-customization-controller/pkg/version"
	// +kubebuilder:scaffold:imports
)

var (
	log = ctrl.Log.WithName("static-server")
)

func loadStaticNMState(fsys fs.FS, env *env.EnvInputs, nmstateDir string, imageServer imagehandler.ImageHandler) error {
	registries, err := env.RegistriesConf()
	if err != nil {
		return err
	}

	// If not defined via env var, look for the mounted secret file
	pullSecret := env.IronicAgentPullSecret
	if env.IronicAgentPullSecret == "" {
		pullSecretRaw, err := fs.ReadFile(fsys, "run/secrets/pull-secret")
		if err != nil {
			return errors.Wrap(err, "unable to read secret")
		}
		pullSecret = string(pullSecretRaw)
	}

	nmstateDir = strings.Trim(nmstateDir, "/")
	files, err := fs.ReadDir(fsys, nmstateDir)
	if err != nil {
		return errors.WithMessagef(err, "problem reading %s", nmstateDir)
	}

	additionalNTPServers := []string{}
	if env.AdditionalNTPServers != "" {
		additionalNTPServers = strings.Split(env.AdditionalNTPServers, ",")
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(nmstateDir, f.Name()))
		if err != nil {
			return errors.WithMessagef(err, "problem reading %s", path.Join(nmstateDir, f.Name()))
		}
		hostname := strings.TrimSuffix(f.Name(), path.Ext(f.Name()))
		igBuilder, err := ignition.New(b, registries,
			env.IronicBaseURL,
			env.IronicInspectorBaseURL,
			env.IronicAgentImage,
			pullSecret,
			env.IronicRAMDiskSSHKey,
			env.IpOptions,
			env.HttpProxy,
			env.HttpsProxy,
			env.NoProxy,
			hostname,
			env.IronicAgentVlanInterfaces,
			additionalNTPServers,
			env.CaBundle,
		)
		if err != nil {
			return errors.WithMessage(err, "failed to configure ignition")
		}
		if err, _ := igBuilder.ProcessNetworkState(); err != nil {
			return errors.WithMessage(err, "failed to convert nmstate data")
		}
		ign, err := igBuilder.Generate()
		if err != nil {
			return errors.WithMessagef(err, "problem generating ignition %s", f.Name())
		}

		for _, suffix := range []string{".iso", ".initramfs"} {
			imageName := strings.TrimSuffix(f.Name(), ".yaml") + suffix

			isInitramfs := !strings.HasSuffix(imageName, ".iso")
			url, err := imageServer.ServeImage(imageName, ign, isInitramfs, true)
			if err != nil {
				return err
			}
			log.Info("serving", "image", imageName, "url", url)
		}
	}
	return nil
}

func main() {
	var devLogging bool
	var imagesBindAddr string
	var imagesPublishAddr string
	var nmstateDir string

	flag.StringVar(&imagesBindAddr, "images-bind-addr", ":8084",
		"The address the images endpoint binds to.")
	flag.StringVar(&imagesPublishAddr, "images-publish-addr", "http://127.0.0.1:8084",
		"The address clients would access the images endpoint from.")
	flag.StringVar(&nmstateDir, "nmstate-dir", "",
		"location of static nmstate files (named with the target image - master-0.yaml).")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(devLogging)))

	version.Print(log)

	env, err := env.New()
	if err != nil {
		log.Error(err, "environment not provided")
		os.Exit(1)
	}

	publishURL, err := url.Parse(imagesPublishAddr)
	if err != nil {
		log.Error(err, "imagesPublishAddr is not parsable")
		os.Exit(1)
	}

	if nmstateDir == "" {
		log.Info("no nmstate-dir provided")
		os.Exit(1)
	}

	imageServer := imagehandler.NewImageHandler(ctrl.Log.WithName("ImageHandler"), env.DeployISO, env.DeployInitrd, publishURL)
	http.Handle("/", http.FileServer(imageServer.FileSystem()))

	if err := loadStaticNMState(os.DirFS("/"), env, nmstateDir, imageServer); err != nil {
		log.Error(err, "problem loading static ignitions")
		os.Exit(1)
	}

	server := http.Server{
		Addr:              imagesBindAddr,
		ReadHeaderTimeout: 5 * time.Second,
	}

	err2 := server.ListenAndServe()

	if err2 != nil {
		log.Error(err, "problem serving images")
		os.Exit(1)
	}
}
