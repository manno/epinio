# Epinio, CLI configuration

Opinionated platform that runs on Kubernetes, that takes you from App to URL in one step.

![CI](https://github.com/epinio/epinio/workflows/CI/badge.svg)

<img src="./docs/epinio.png" width="50%" height="50%">

The Epinio command line client uses a configuration file to store
information which has to persist across invocations. This document
discusses various aspects around this.

## Table Of Contents

  - [Location](#location)
  - [Contents](#contents)
  - [Commands](#commands)

## Location

Epinio's configuration is located by default at `~/.config/epinio/config.yaml`.

The location can be changed from command to command by specifying a
different path with the global command line option `--config-file`.

A more permanent change is possible by setting the environment
variable `EPINIO_CONFIG` to the desired path.

## Contents

Epinio's configuration contains

  - The name of the organization currently targeted.
  - Epinio API user name
  - Epinio API password
  - Epinio API certificate
  
The organization can be changed by running `epoinio target` with the
name of the desired org as its single argument.

User name and password are used by the client to authenticate itself
when talking to Epinio's API server. The `epinio install` command
saves the initial information to the configuration.

The certificate information is set only when installing epinio for
development. This uses an `omg.howdoi.website` domain with a
self-signed certificate. `epinio install` saves the associated CA
certificate to the configuration so that future invocations of the
client are able to verify the actual certificate when talking to
Epinio's API server.

## Commands

The Epinio command line client currently provides 3 commands
explicitly targeting the configuration. These are:

  1. `epinio target`

     As noted in the previous section, this command changes
     organization to target with all other commands.

  2. `epinio config show`

     This command shows the details of the currently stored
     configuration settings. An exception is made for the certificate
     field, due to its expected size. The command's output only notes
     if certificate data is present or not.

  3. `epinio config update-credentials`

     Epinio allows users to switch between multiple installations (on
     different clusters) by simply re-targeting the cluster to talk to
     via changes to the  environment variable `KUBECONFIG`.

     When such is done the credentials and cert data stored in the
     configuration will not match the newly targeted cluster, except
     by coincidence.

     To be actually able to talk to the newly targeted installation it
     is necessary to run this command to refresh the stored
     credentials and cert data with information retrieved from the new
     cluster.

