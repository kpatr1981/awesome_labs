package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"

	"awesome_labs/internal/lab"
)

// The harness image stands in for the kl-docker VM: Ubuntu 24.04 with systemd
// as PID 1 and the Docker engine at the versions the VM pins, the packages the
// VM's template and docker bootstrap install, and a lab user with passwordless
// sudo in the docker group. Keep the package lists in step with
// vms/docker.yaml and vms/bootstrap/docker.sh.
var (
	// vms/docker.yaml's apt-get line
	templatePkgs = "ca-certificates curl gpg jq vim nano less tree git make bash-completion " +
		"dnsutils net-tools iproute2 iptables lsof strace tcpdump htop openssl apache2-utils"
	// vms/bootstrap/docker.sh's PKGS
	bootstrapPkgs = "strace lsof lvm2 dnsmasq-base wireguard-tools traceroute acl"
	// what Ubuntu's cloud image has and its container image lacks
	cloudPkgs = "sudo python3 iputils-ping openssh-client cron rsync wget unzip zip file procps psmisc " +
		"kmod util-linux xz-utils bzip2 netcat-openbsd e2fsprogs locales tzdata man-db logrotate python3-yaml"
)

// LabUser is the lab user's name inside the harness.
const LabUser = "lab"

// Hostname is the docker VM's hostname, which the harness container takes too.
var Hostname = lab.DockerEnv.Primary().Hostname

func dockerfile() string {
	return fmt.Sprintf(`FROM ubuntu:24.04
ENV container=docker DEBIAN_FRONTEND=noninteractive
RUN set -eux; apt-get update; \
    apt-get install -y --no-install-recommends systemd systemd-sysv dbus %[1]s %[2]s %[3]s; \
    install -m 0755 -d /etc/apt/keyrings; \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc; \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu noble stable" >/etc/apt/sources.list.d/docker.list; \
    apt-get update; \
    apt-get install -y docker-ce='%[4]s' docker-ce-cli='%[4]s' containerd.io='%[5]s' docker-buildx-plugin='%[6]s' docker-compose-plugin='%[7]s'; \
    rm -rf /var/lib/apt/lists/*; \
    mkdir -p /etc/docker; echo '{"registry-mirrors": ["https://mirror.gcr.io"]}' >/etc/docker/daemon.json; \
    useradd -m -s /bin/bash -G docker,sudo %[8]s; \
    echo '%[8]s ALL=(ALL) NOPASSWD:ALL' >/etc/sudoers.d/%[8]s; chmod 440 /etc/sudoers.d/%[8]s; \
    systemctl enable docker containerd; \
    systemctl mask getty.target console-getty.service systemd-firstboot.service systemd-udevd.service systemd-udevd-kernel.socket systemd-udevd-control.socket; \
    mkdir -p /lab /var/lib/kubelab /opt/kubelab
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
`, templatePkgs, bootstrapPkgs, cloudPkgs, lab.DockerVersion, lab.ContainerdVersion,
		lab.BuildxVersion, lab.ComposeVersion, LabUser)
}

// imageTag names the harness image after its Dockerfile, so a change rebuilds it.
func imageTag() string { return "kl-local-host:" + short(dockerfile()) }

// storeHash identifies a prepared image store: the harness image plus what the
// docker bootstrap pulls and builds.
func storeHash(assets fs.FS) (string, error) {
	h := sha256.New()
	h.Write([]byte(dockerfile()))
	for _, f := range []string{"vms/bootstrap/common.sh", "vms/bootstrap/docker.sh", "vms/kit/images.txt"} {
		b, err := fs.ReadFile(assets, f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func short(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// sanitize makes an exercise id usable in a container name.
func sanitize(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '-'
	}, id)
}
