# LXD Runtime Setup

Guide for setting up the LXD runtime backend on a host that runs the arca server.

## Prerequisites

- LXD (snap or system package)
- `lxc` CLI available in `$PATH`

## 1. Storage Pool

LXD requires a storage pool attached to the default profile. Without one, container creation fails with `No root device could be found`.

```bash
lxc storage create default dir
lxc profile device add default root disk path=/ pool=default
```

> The `dir` backend is simplest. For production, consider `zfs` or `btrfs` for snapshot and copy performance.

## 2. Network

Containers need a managed bridge network for internet access (package installation, etc.).

```bash
lxc network create lxdbr0 ipv4.address=10.200.0.1/24 ipv4.nat=true ipv6.address=none
lxc profile device add default eth0 nic network=lxdbr0 name=eth0
```

If auto-assignment fails (e.g. subnet conflicts with Docker's `172.17.0.0/16`), specify an explicit subnet as shown above.

## 3. Docker Coexistence

If Docker is installed on the same host, LXD containers will have no outbound connectivity by default. This is because Docker sets the iptables FORWARD chain policy to `DROP`, which blocks traffic from LXD's bridge even though LXD has its own nftables ACCEPT rules.

### Why This Happens

The Linux kernel evaluates both iptables (`ip filter`) and nftables (`inet lxd`) FORWARD hooks. A packet must be accepted by **all** of them. Docker's `DROP` policy blocks LXD traffic before LXD's own rules can apply.

This affects any bridge-based networking (LXD, libvirt, etc.) running alongside Docker.

### Fix

Add rules to the `DOCKER-USER` chain, which Docker provides for exactly this purpose:

```bash
sudo iptables -I DOCKER-USER -i lxdbr0 -j ACCEPT
sudo iptables -I DOCKER-USER -o lxdbr0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
```

### Persisting the Rules

These iptables rules are lost on reboot. Create a systemd unit to restore them after Docker starts:

```bash
sudo tee /etc/systemd/system/lxd-docker-compat.service > /dev/null << 'EOF'
[Unit]
Description=Allow LXD traffic through Docker FORWARD chain
After=docker.service lxd.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/sbin/iptables -I DOCKER-USER -i lxdbr0 -j ACCEPT
ExecStart=/usr/sbin/iptables -I DOCKER-USER -o lxdbr0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable lxd-docker-compat.service
```

Alternatively, use `iptables-persistent`:

```bash
sudo apt-get install -y iptables-persistent
sudo netfilter-persistent save
```

## Verification

After setup, verify the default profile has both storage and network:

```bash
$ lxc profile show default
name: default
devices:
  eth0:
    name: eth0
    network: lxdbr0
    type: nic
  root:
    path: /
    pool: default
    type: disk
```

Test container connectivity:

```bash
lxc launch ubuntu:24.04 test-container
lxc exec test-container -- ping -c 2 8.8.8.8
lxc delete test-container --force
```
