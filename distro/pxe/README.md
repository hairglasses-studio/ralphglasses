# PXE Boot Configuration

Network boot the ralphglasses thin client from your UNRAID or Proxmox server.

## Overview

PXE boot eliminates local storage — the thin client boots entirely from the network.
The server serves an x86_64 thin-client image + ralphglasses customizations.

## Setup

### Option 1: LTSP (Linux Terminal Server Project)

```bash
# On server (UNRAID Docker or Proxmox VM)
apt install ltsp
ltsp image  # generates squashfs from server install
ltsp ipxe   # generates iPXE boot files
ltsp dnsmasq --proxy-dhcp=yes  # DHCP for PXE
```

### Option 2: ThinStation

ThinStation is purpose-built for PXE thin clients (~50MB).

### Requirements

- DHCP server with PXE options (next-server, filename)
- TFTP server serving boot images
- NFS or HTTP server for root filesystem
- Network switch between server and thin clients

## TODO

- [ ] Build LTSP image from the supported x86_64 thin-client rootfs
- [ ] Create ThinStation build config
- [ ] Test with actual thin client hardware
- [ ] Document UNRAID Docker container for PXE server
