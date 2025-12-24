# AGENTS.md - AI Agent Guidelines for Image Customization Controller

## Project Overview

This is the **OpenShift Image Customization Controller** - a Kubernetes controller that reconciles [Metal³](https://metal3.io)'s `PreprovisioningImage` custom resources. It builds CoreOS live images customized with Ignition configurations to start the Ironic Python Agent (IPA), including per-host network data in [NMState](https://nmstate.io) format. Images are served from a built-in HTTP webserver.

**Repository**: `github.com/openshift/image-customization-controller`

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Image Customization Controller                    │
├─────────────────────────────────────────────────────────────────────┤
│  PreprovisioningImage CR  ──►  ImageProvider  ──►  ImageHandler     │
│         (Metal3)                 (RHCOS)            (HTTP Server)   │
│              │                      │                    │          │
│              ▼                      ▼                    ▼          │
│         NMState Data    ──►   Ignition Builder  ──►  ISO/Initramfs  │
└─────────────────────────────────────────────────────────────────────┘
```

### Binaries

1. **`/machine-image-customization-controller`** (`cmd/controller/main.go`)
   - Main Kubernetes controller
   - Reconciles `PreprovisioningImage` resources
   - Ignores resources with label `infraenvs.agent-install.openshift.io`

2. **`/machine-image-customization-server`** (`cmd/static-server/main.go`)
   - Standalone HTTP server for static configurations
   - Uses filesystem-based NMState files instead of Kubernetes resources

## Directory Structure

```
.
├── cmd/
│   ├── controller/main.go     # Kubernetes controller entrypoint
│   └── static-server/main.go  # Static file server entrypoint
├── pkg/
│   ├── env/                   # Environment variable configuration
│   ├── ignition/              # Ignition config generation
│   │   ├── builder.go         # Main ignition builder
│   │   ├── nmstate.go         # NMState to NetworkManager conversion
│   │   ├── service_config.go  # IPA service & config generation
│   │   └── file_embed.go      # File embedding utilities
│   ├── imagehandler/          # HTTP filesystem for image serving
│   │   ├── imagehandler.go    # Main handler interface
│   │   ├── basefile.go        # ISO/initramfs base file handling
│   │   ├── imagefile.go       # Virtual image file implementation
│   │   └── imagefilesystem.go # HTTP filesystem implementation
│   ├── imageprovider/         # Metal3 ImageProvider implementation
│   │   └── rhcos.go           # RHCOS-specific image provider
│   └── version/               # Version information
├── vendor/                    # Vendored dependencies
├── Dockerfile                 # Container build
├── Makefile                   # Build automation
└── example.yaml               # Example K8s resources
```

## Key Packages

### `pkg/env`
Handles environment configuration via `envconfig`. Required variables:
- `DEPLOY_ISO` - Path to CoreOS base ISO
- `DEPLOY_INITRD` - Path to CoreOS initramfs
- `IRONIC_AGENT_IMAGE` - IPA container image pullspec

Optional variables: `IRONIC_BASE_URL`, `IRONIC_INSPECTOR_BASE_URL`, `IRONIC_AGENT_PULL_SECRET`, `IRONIC_RAMDISK_SSH_KEY`, `REGISTRIES_CONF_PATH`, `IP_OPTIONS`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, `ADDITIONAL_NTP_SERVERS`, `CA_BUNDLE`

### `pkg/ignition`
Generates Ignition v3.2 configurations containing:
- IPA service unit (`ironic-agent.service`)
- IPA configuration file (`/etc/ironic-python-agent.conf`)
- NetworkManager connection files (from NMState)
- Optional: SSH keys, registries config, CA bundles, NTP servers

Uses `nmstatectl gc` for NMState to NetworkManager conversion (requires `nmstate` package).

### `pkg/imagehandler`
Implements `http.FileSystem` for serving customized images:
- Supports both ISO and initramfs formats
- Uses `assisted-image-service/pkg/isoeditor` for image manipulation
- Generates images on-the-fly by overlaying Ignition on base images
- Supports HTTP Range requests
- Multi-architecture support via filename patterns

### `pkg/imageprovider`
Implements Metal3's `imageprovider.ImageProvider` interface for RHCOS images.

## Development

### Prerequisites
- Go 1.23+
- `nmstate` package (for `nmstatectl`)
- Docker (for container builds)

### Build Commands
```bash
# Build binaries
make image-customization-controller
make image-customization-server

# Run tests
make test        # lint + unit tests
make unit        # unit tests only
make lint        # golangci-lint

# Build container
make docker
```

### Testing
Tests use the `testify` package. Run with:
```bash
go test ./... -coverprofile cover.out
# Or with verbose output:
make VERBOSE=-v unit
```

### Linting
Uses vendored `golangci-lint v2`. Configuration is in the default location. Run:
```bash
make lint
# Auto-fix issues:
make generate
```

## Code Conventions

### Error Handling
- Use `github.com/pkg/errors` for wrapping errors with context
- Return `imageprovider.BuildInvalidError` for user-fixable configuration errors

### Logging
- Use `logr.Logger` from `github.com/go-logr/logr`
- Obtain loggers via `ctrl.Log.WithName("component")`

### Kubernetes Patterns
- Uses `controller-runtime` for reconciliation
- Scheme registration in `init()`
- Cache with label selectors for filtering resources

### Ignition Generation
- Ignition version 3.2.0
- Files embedded as data URLs
- Use `ignitionFileEmbed()` for creating files
- Use `ignitionFileEmbedAppend()` for appending to files

## Important Implementation Details

1. **Image URLs are random** - Generated UUIDs change on controller restart
2. **Only Ignition stored** - Full images generated on-demand from base + ignition
3. **Label filtering** - Resources with `infraenvs.agent-install.openshift.io` label are ignored
4. **VLAN interface behavior** - Controlled by `IRONIC_AGENT_VLAN_INTERFACES` (always/never/auto)
5. **Multi-arch support** - Detects architecture from filename patterns like `image_x86_64.iso` or `image.aarch64.iso`

## Dependencies

Key external dependencies:
- `github.com/metal3-io/baremetal-operator` - Controller framework and CR types (vendored from OpenShift fork)
- `github.com/coreos/ignition/v2` - Ignition config types
- `github.com/openshift/assisted-image-service` - ISO/initramfs editing
- `sigs.k8s.io/controller-runtime` - Kubernetes controller framework

## Common Tasks for AI Agents

### Adding a new environment variable
1. Add field to `EnvInputs` struct in `pkg/env/env.go`
2. Update `ignitionBuilder` in `pkg/ignition/builder.go` if used in ignition
3. Pass through the builder chain: `New()` → `GenerateConfig()` → usage
4. Update README.md with documentation

### Modifying Ignition output
1. Edit `GenerateConfig()` in `pkg/ignition/builder.go`
2. Use `ignitionFileEmbed()` for new files
3. Add to `config.Storage.Files` or `config.Systemd.Units`
4. Add tests in `builder_test.go`

### Adding new image format support
1. Implement `baseFile` interface in `pkg/imagehandler/basefile.go`
2. Add detection logic in `loadOSImage()` in `imagehandler.go`
3. Update `SupportsFormat()` in `pkg/imageprovider/rhcos.go`

### Testing changes
1. Write unit tests adjacent to implementation (`*_test.go`)
2. Use `testify/assert` and `testify/require`
3. Run `make test` before committing

## Contact

See `OWNERS` file for maintainers:
- Component: Bare Metal Hardware Provisioning
- Subcomponent: OS Image Provider
