> [!WARNING]
> This Repository is under development and not ready for productive use. It is in an alpha stage. That means APIs and concepts may change on short notice including breaking changes or complete removal of apis.

# Platform Mesh - rebac-authz-webhook
![Build Status](https://github.com/platform-mesh/rebac-authz-webhook/actions/workflows/pipeline.yml/badge.svg)

## Description

The Platform Mesh IAM Authorizaton Webhook is a kubernetes authorization webhook that uses openFGA to answer authorization requests from kubernetes.

## Releasing

The release is performed automatically through a GitHub Actions Workflow.

All the released versions will be available as packages on this GitHub repository.

## Requirements

To build an run the webhook locally a installation of go is required. Checkout the [go.mod](go.mod) for the required go version.
In order to build the Dockerfile a compatible tooling like docker or podman is required.

## Contributing

Please refer to the [CONTRIBUTING.md](CONTRIBUTING.md) file in this repository for instructions on how to contribute to Platform Mesh.

## Code of Conduct

Please refer to the [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) file in this repository informations on the expected Code of Conduct for contributing to Platform Mesh.

## Licensing

Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available via the [REUSE tool](https://api.reuse.software/info/github.com/platform-mesh/rebac-authz-webhook). 
