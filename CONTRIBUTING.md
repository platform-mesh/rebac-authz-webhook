## Overview

# Contributing to Platform Mesh
We want to make contributing to this project as easy and transparent as possible.

## Our development process
We use GitHub to track issues and feature requests, as well as accept pull requests.

## Pull requests
You are welcome to contribute with your pull requests. These steps explain the contribution process:

1. Fork the repository and create your branch from `main`.
1. [Add tests](#testing) for your code.
1. If you've changed APIs, update the documentation. 
1. Make sure the tests pass. Our github actions pipeline is running the unit and e2e tests for your PR and will indicate any issues.
1. Sign the Developer Certificate of Origin (DCO).

## Testing

> **NOTE:** You should always add if you are adding code to our repository.

To let tests run locally, run `go test ./...` in the root directory of the repository.

## Mocks

golang commons uses mockery for mocking. If mock is absent, you can update `.mockery.yaml` file by adding the missing mock. Then run the following command to generate the mock files:
```
task mockery
``` 
P.S. If you have golang installed, it automatically installs the mockery binary in `golang-commons/bin` directory.

## Issues
We use GitHub issues to track bugs. Please ensure your description is
clear and includes sufficient instructions to reproduce the issue.

## License
By contributing to Platform Mesh, you agree that your contributions will be licensed
under its [Apache-2.0 license](LICENSE).