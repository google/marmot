# Marmot
## One line summary
Marmot is a service for processing workflows targeting DevOps/SRE needs.
## The long summary
Marmot is a GRPC service that executes workflow descriptions against
infrastructure (network devices, servers, kubernetes pods, ...).
This allows top level services/scripts to simply test the output for the correct
workflow descriptors instead of complex mocking, concurrency checks, ...
This in turn provides code reuse and reduces code duplication. It also provides
safety by having a single system responsible for execution and not hundreds of
scripts/services.
Mamort provides:
* Structured workflow description language with health checks
* Support for concurrency inside workflows
* Plugin architecture allows feature expansion/updates without service rebuilds
* Streaming execution updates
* Clients for Go and Python
* Support for emergency pausing or stopping of all workflows, classes of
  workflows or single workflows
* Web UI for viewing workflows
Marmot is based on an internal Google project which processes tens of thousands
of workflows per week for several internal SRE/DevOps organizations.
## Use cases
Marmot has been designed as a DevOps/SRE tool for handling
infrastructure changes, though it is not limited to this role.  Marmot is well
suited for any type of operation that must be performed in steps with certain
pacing and may require state checks for health.
Examples include:
* Updating packages on servers
* Rolling out a new service version on Kubernetes
* Configuration changes to routing infrastructure
* Updating firmware on devices
* Turning up new devices via a mix of BOOTP/Console/SSH
* Automatic acceptance of code changes into a master repository from staging
* ...
## Disclaimers
This is not an official Google product.
