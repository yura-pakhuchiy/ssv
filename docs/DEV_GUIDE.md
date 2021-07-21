[<img src="./resources/bloxstaking_header_image.png" >](https://www.bloxstaking.com/)

<br>
<br>

# SSV - Development Guide

* [Usage](#usage)
  + [Common Commands](#common-commands)
    - [Build](#build)
    - [Test](#test)
    - [Lint](#lint)
    - [Specify Version](#specify-version)
    - [Splitting a Validator Key](#splitting-a-validator-key)
    - [Generating an Operator Key](#generating-an-operator-key)
  + [Config Files](#config-files)
    - [Node Config](#node-config)
    - [Shares Config](#shares-config)
* [Running a Local Network of Operators](#running-a-local-network-of-operators)
  + [Install](#install)
    - [Prerequisites](#prerequisites)
    - [Clone Repository](#clone-repository)
    - [Build Binary](#build-binary)
  + [Configuration](#configuration)
    - [Split Validator Key](#split-validator-key)
    - [Create Config Files](#create-config-files)
      * [Node Config](#node-config-1)
      * [Shares Config](#shares-config-1)
  + [Run](#run)
    - [Local network with 4 nodes with Docker Compose](#local-network-with-4-nodes-with-docker-compose)
    - [Local network with 4 nodes for debugging with Docker Compose](#local-network-with-4-nodes-for-debugging-with-docker-compose)
    - [Local network with 4 nodes with Podman](#local-network-with-4-nodes-with-podman)
* [Coding Standards](#coding-standards)

## Usage

### Common Commands

#### Build
```bash
$ make build
```

#### Test
```bash
$ make full-test
```

#### Lint
```bash
$ make lint-prepare
$ make lint
```

#### Specify Version

```bash
$ ./bin/ssvnode version
```

#### Splitting a Validator Key

We split an eth2 BLS validator key into shares via Shamir-Secret-Sharing(SSS) to be used between the SSV nodes.

```bash
# Extract Private keys from mnemonic (optional, skip if you have the public/private keys ) 
$ ./bin/ssvnode export-keys --mnemonic="<mnemonic>" --index={keyIndex}

# Generate threshold keys
$ ./bin/ssvnode create-threshold --count <number of ssv nodes> --private-key <privateKey>
```

#### Generating an Operator Key

```bash
$ ./bin/ssvnode generate-operator-keys
```

### Config Files

Config files are located in `./config` directory:

#### Node Config 

Specifies general configuration regards the current node. \
Example yaml - [config.yaml](../config/config.yaml)

#### Shares Config

For a 4 node SSV network, 4 share<nodeId>.yaml files need to be created, based on the [template file](../config/example_share.yaml). \
E.g. `./config/share1.yaml`, `./config/share2.yaml`, etc.

## Running a Local Network of Operators

This section details the steps to run a local network of operator nodes.

### Install

#### Prerequisites

In order to run a local environment, install the following:
* git
* go (1.15)
* docker
* make

#### Clone Repository

```shell
$ git clone https://github.com/bloxapp/ssv.git
```

#### Build Binary

```shell
$ make build
```

### Configuration

#### Split Validator Key

Split a validator key to distribute to the nodes in your network. \
See [Splitting a Validator Key](#splitting-a-validator-key).

#### Create Config Files

##### Node Config

Fill the required fields in [config.yaml](../config/config.yaml) file. \
Note - there's no need to fill the OperatorPrivateKey field.

##### Shares Config

Create 4 .yaml files with the corresponding configuration, based on the [template file](../config/example_share.yaml). \
The files should be placed in the `./config` directory (`./config/share1.yaml`, `./config/share2.yaml`, etc.)


### Run

Run a local network using `docker`

#### Local network with 4 nodes with Docker Compose

```shell
$ make docker-all 
```

#### Local network with 4 nodes for debugging with Docker Compose

```shell
$ make docker-debug 
```

#### Local network with 4 nodes with Podman

[Podman](https://podman.io) is a daemonless container engine for developing, managing,
and running OCI Containers on Linux systems. Containers can either be run as root or in rootless mode.


On MacOS / Windows - need to run a remote client that connects to a Linux VM where Podman is running.

Install the following:
* [VirtualBox](https://www.virtualbox.org/wiki/Downloads)
* [Vagrant](https://www.vagrantup.com/downloads)
  * disksize plugin: \
    `$ vagrant plugin install vagrant-disksize`
* [Podman](https://podman.io/getting-started/installation)

Once setup is done, spin up a VM from the root folder of this project:

```shell
$ vagrant up
```

It will mount the folder to `/vagarnt/ssv` on the VM.

On your desktop run the following to add a client:
```shell
$ export CONTAINER_HOST=ssh://vagrant@127.0.0.1:2222/run/user/1000/podman/podman.sock
$ export CONTAINER_SSHKEY=$PWD/.vagrant/machines/default/virtualbox/private_key
$ podman system connection add --identity "$CONTAINER_SSHKEY" vagrant $CONTAINER_HOST
```
Check that podman works
```shell
$ podman -c vagrant info
```

On Linux:
* [Podman](https://podman.io/getting-started/installation)

Podman is compatible with docker, you can `alias docker=podman`.



## Coding Standards

Please make sure your contributions adhere to our coding guidelines:

* Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
  guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
* Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
  guidelines.
* Pull requests need to be based on and opened against the `stage` branch, and its commits should be squashed on merge.
