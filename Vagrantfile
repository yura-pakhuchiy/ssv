# -*- mode: ruby -*-
# vi: set ft=ruby :

# setup a linux machine for podman, based on:
# https://github.com/containers/podman/issues/6809#issuecomment-662031057
Vagrant.configure("2") do |config|

  config.vm.box = "ubuntu/focal64"

  config.disksize.size = '40GB'

  config.vm.synced_folder ".", "/vagrant", disabled: true
  config.vm.synced_folder ".", "/vagrant/ssv"

  config.vm.provider "virtualbox" do |vb|
    vb.memory = "2048"
    vb.cpus = 2
  end

  config.vm.provision "shell", privileged: false, inline: <<-SHELL
    export DEBIAN_FRONTEND=noninteractive
    sudo apt-get update -y

    # install podman
    # for ubuntu <= 20.04: need to set apt on latest version
    source /etc/os-release
    sudo sh -c "echo 'deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_${VERSION_ID}/ /' > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list"
    wget -nv https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable/xUbuntu_${VERSION_ID}/Release.key -O- | sudo apt-key add -
    sudo apt-get update -qq
    # and install with apt
    sudo apt-get install -qq -y podman

    # install dependencies
    sudo apt-get install -qq -y make g++ python gcc-aarch64-linux-gnu \
        apt-transport-https lsb-release ca-certificates gnupg git zip unzip bash curl 2> /dev/null
    # manually installing yq due to versioning issues
    sudo wget -O /usr/local/bin/yq https://github.com/mikefarah/yq/releases/download/3.3.0/yq_linux_amd64
    sudo chmod +x /usr/local/bin/yq
    # install go
    sudo snap install --classic --channel=1.15/stable go

    # enable rootless socket, for podman client on the host machine
#     systemctl enable --user podman.socket
#     systemctl start --user podman.socket
  SHELL
end
