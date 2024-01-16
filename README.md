# Machine Image Customization Controller

This repo contains a controller that reconciles [Metal³](https://metal3.io)'s
`PreprovisioningImage` custom resources. The image built is a CoreOS live image
customized with an Ignition file to start the Ironic Python Agent (IPA) and
containing any per-host network data provided in [NMState](https://nmstate.io)
format. Images are served from a webserver built in to the controller.

The main reconciler loop is vendored from the generic Metal³ implementation.
Only a custom `ImageProvider` plugin is implemented here.

## Image building

Network data for each host must be in NMState format, under a key named
`nmstate` in the Secret specified by the `networkDataName` field in the
`PreprovisioningImage`.

Note that all `PreprovisioningImage`s with the label
`infraenvs.agent-install.openshift.io` will be ignored by this controller.

Generated URLs are random and will change when the controller is restarted.

Only the Ignition file for each image is stored. When an HTTP request is
received, the web server generates a stream on the fly with a CPIO archive
containing the Ignition file overlaid on the appropriate portion of the ISO or
appended to the initramfs. HTTP Range requests are supported.

## How to run

### Environment

The following environment variables are required:

- `IRONIC_AGENT_IMAGE` --- Pullspec for the IPA container image
- `DEPLOY_ISO` --- Filesystem path to the CoreOS base ISO
- `DEPLOY_INITRD` --- Filesystem path to the CoreOS initramfs

The following environment variables can also be set to customize the content of
the Ignition:

- `IRONIC_BASE_URL`
- `IRONIC_INSPECTOR_BASE_URL`
- `IRONIC_AGENT_PULL_SECRET`
- `IRONIC_AGENT_VLAN_INTERFACES`
- `IRONIC_RAMDISK_SSH_KEY`
- `REGISTRIES_CONF_PATH`
- `IP_OPTIONS`
- `HTTP_PROXY`
- `HTTPS_PROXY`
- `NO_PROXY`

### Running the Controller

The controller binary is `/machine-image-customization-controller`.

The following command line flags are used for configuration:

- `-namespace` --- Namespace that the controller watches to reconcile
  preprovisioningimage resources. (Defaults to `$WATCH_NAMESPACE`; if not set
  watches all namespaces.)
- `-images-bind-addr` --- The address and port for the web server to bind to.
  (Defaults to `:8084`.)
- `-images-publish-addr` --- The address clients would access the images
  endpoint from. (Defaults to `http://127.0.0.1:8084`.)

### Running statically

There is also a separate binary, `/machine-image-customization-server`, that
runs the web server using static config files, instead of as a Kubernetes
controller.

The following command line flags are used for configuration:

- `-nmstate-dir` --- Location of static NMState files (named with the target
  image, e.g. `worker-0.yaml`).
- `-images-bind-addr` --- The address and port for the web server to bind to.
  (Defaults to `:8084`.)
- `-images-publish-addr` --- The address clients would access the images
  endpoint from. (Defaults to `http://127.0.0.1:8084`.)

An NMState file named `<nmstate-dir>/worker-0.yaml` will be built into images
published at `<images-publish-addr>/worker-0.iso` and
`<images-publish-addr>/worker-0.initramfs`.
