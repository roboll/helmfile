Vagrant.configure("2") do |config|
  config.vm.box = "ubuntu/focal64"
  config.vm.hostname = "minikube.box"
  config.vm.provision :shell, privileged: false,
    inline: <<-EOS
      set -e
      sudo apt-get update
      sudo apt-get install -y make docker.io
      sudo systemctl start docker
      sudo usermod -G docker $USER
      cd /vagrant/.circleci
      make all
    EOS

  config.vm.provider "virtualbox" do |v|
    v.memory = 2048
    v.cpus = 2
  end
end
