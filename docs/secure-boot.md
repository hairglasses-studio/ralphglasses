# Secure Boot Support

Ralphglasses thin clients can boot with UEFI Secure Boot enabled. This requires signing the kernel with a Machine Owner Key (MOK) and enrolling that key in the shim bootloader's trust database.

Two scripts handle the workflow:

- `distro/secureboot/sign-kernel.sh` — generates a MOK key pair and signs the kernel
- `distro/secureboot/mok-enroll.sh` — enrolls the MOK so the signed kernel is trusted at boot

## Prerequisites

### Packages

**Debian / Ubuntu:**

```bash
sudo apt install sbsigntool mokutil openssl
```

**RHEL / Fedora:**

```bash
sudo dnf install sbsigntools mokutil openssl
```

### UEFI Settings

Secure Boot must be enabled in the motherboard firmware. For the ASUS ProArt X870E-CREATOR WIFI:

1. Enter UEFI setup (press DEL during POST)
2. Navigate to **Boot > Secure Boot**
3. Set **Secure Boot** to **Enabled**
4. Set **Secure Boot Mode** to **Custom** (allows MOK enrollment)
5. Save and reboot

Verify from Linux:

```bash
mokutil --sb-state
# Expected: SecureBoot enabled
```

## Key Generation

The `sign-kernel.sh` script automatically generates a 4096-bit RSA key pair on first run if none exists. Keys are stored in `/var/lib/ralphglasses/mok/` by default.

| File | Format | Purpose |
|------|--------|---------|
| `MOK.key` | PEM | Private key (0600 permissions, never leaves the machine) |
| `MOK.crt` | PEM | X.509 certificate (for sbsign) |
| `MOK.der` | DER | X.509 certificate (for mokutil enrollment) |

The certificate is valid for 10 years and includes the `codeSigning` extended key usage extension.

To override the key directory or certificate common name:

```bash
export RALPH_MOK_DIR=/path/to/keys
export RALPH_MOK_CN="my custom MOK name"
```

To generate keys without signing a kernel:

```bash
sudo sign-kernel.sh --dry-run   # preview
sudo sign-kernel.sh              # generates keys, then signs kernel
```

## Kernel Signing

### Sign the running kernel

```bash
sudo distro/secureboot/sign-kernel.sh
```

### Sign a specific kernel image

```bash
sudo distro/secureboot/sign-kernel.sh /boot/vmlinuz-6.8.0-45-generic
```

### What happens

1. If no MOK key pair exists, one is generated
2. The kernel image is signed with `sbsign` using the MOK private key
3. The signature is verified with `sbverify`
4. The original kernel is backed up to `vmlinuz-*.unsigned`
5. The signed kernel replaces the original at the same path

### Dry run

```bash
sudo distro/secureboot/sign-kernel.sh --dry-run
```

Prints all actions without modifying any files.

## MOK Enrollment

After signing the kernel, the MOK certificate must be enrolled in the shim bootloader's trust database. This is a two-phase process: an import request from Linux, followed by confirmation at the shim prompt during reboot.

### Phase 1: Import request

```bash
sudo distro/secureboot/mok-enroll.sh
```

You will be prompted to set a **one-time enrollment password**. Remember it for the next step.

### Phase 2: Reboot and confirm

1. Reboot the machine
2. The shim bootloader displays **Perform MOK management**
3. Select **Enroll MOK**
4. Select **Continue**
5. Enter the enrollment password from Phase 1
6. Select **Reboot**

### Verify enrollment

After the second reboot:

```bash
distro/secureboot/mok-enroll.sh --status
```

Expected output includes `MOK enrolled: true`.

You can also verify directly:

```bash
mokutil --list-enrolled | grep "ralphglasses"
```

## Automated First-Boot Enrollment

For fleet deployments, the enrollment can be partially automated using a systemd service that runs `sign-kernel.sh` and `mok-enroll.sh` on first boot. The enrollment password must still be entered manually at the shim prompt on the first reboot (this is a deliberate UEFI security requirement and cannot be bypassed).

Example systemd unit (`distro/systemd/ralph-secureboot.service`):

```ini
[Unit]
Description=Ralphglasses Secure Boot setup
ConditionPathExists=!/var/lib/ralphglasses/mok-enrolled
After=local-fs.target

[Service]
Type=oneshot
ExecStart=/opt/ralphglasses/distro/secureboot/sign-kernel.sh
ExecStart=/opt/ralphglasses/distro/secureboot/mok-enroll.sh
StandardInput=tty
TTYPath=/dev/console

[Install]
WantedBy=multi-user.target
```

## Troubleshooting

### "Secure Boot: DISABLED"

MOK enrollment succeeds but has no effect until Secure Boot is enabled in UEFI settings. Enable it and reboot.

### "sbsign not found"

Install the signing tools:

```bash
# Debian/Ubuntu
sudo apt install sbsigntool

# RHEL/Fedora
sudo dnf install sbsigntools
```

### "mokutil not found"

```bash
# Debian/Ubuntu
sudo apt install mokutil

# RHEL/Fedora
sudo dnf install mokutil
```

### Signature verification fails after signing

The kernel image may have been modified after signing (e.g., by a kernel postinst hook). Re-run `sign-kernel.sh` to re-sign.

### Shim does not prompt for MOK enrollment on reboot

- Verify a pending request exists: `mokutil --list-new`
- Ensure the system is booting via the shim bootloader (not directly via the UEFI shell)
- Check that Secure Boot mode is set to **Custom** in UEFI settings

### "MOK key is NOT enrolled" after reboot

The enrollment may have been cancelled or the wrong password was entered. Re-run `mok-enroll.sh` and reboot again.

### Permission denied on MOK private key

The private key at `/var/lib/ralphglasses/mok/MOK.key` must be owned by root with 0600 permissions:

```bash
sudo chown root:root /var/lib/ralphglasses/mok/MOK.key
sudo chmod 0600 /var/lib/ralphglasses/mok/MOK.key
```

### DKMS modules (NVIDIA drivers)

Third-party kernel modules (such as NVIDIA drivers installed via DKMS) also need to be signed. Most distributions handle this automatically when MOK is enrolled. If not, sign manually:

```bash
sudo sbsign --key /var/lib/ralphglasses/mok/MOK.key \
             --cert /var/lib/ralphglasses/mok/MOK.crt \
             --output /path/to/module.ko \
             /path/to/module.ko
```

For the dual-RTX 4090 setup on the ProArt X870E, NVIDIA DKMS modules will need signing after each kernel update. Consider adding a kernel postinst hook to automate this.
