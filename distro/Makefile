# distro/Makefile — ISO build pipeline for ralphglasses thin client
#
# Two build paths:
#   1. Docker-based (default): docker → rootfs → iso
#   2. Debootstrap-based:      debootstrap → chroot → iso-debootstrap
#
# Usage:
#   make iso                Build bootable UEFI ISO (Docker path)
#   make iso-debootstrap    Build bootable UEFI ISO (debootstrap path)
#   make docker             Build Docker image only (for testing)
#   make test-vm            Launch ISO in QEMU
#   make usb DEVICE=X       Write ISO to USB drive
#   make validate            Validate built ISO
#   make clean               Remove build artifacts

SHELL := /bin/bash
.PHONY: all iso iso-debootstrap docker rootfs rootfs-debootstrap chroot \
        test-vm usb clean validate validate-grub install-deps check-paths

# ── Variables ──────────────────────────────────────────────────────────
DISTRO_NAME  := ralphglasses
VERSION      := 0.1.0
ARCH         := amd64
IMAGE_NAME   := $(DISTRO_NAME)-os
IMAGE_TAG    := latest
OUTPUT_ISO   := $(DISTRO_NAME)-$(VERSION)-$(ARCH).iso
ISO_NAME     := $(OUTPUT_ISO)
ROOTFS_DIR   := build/rootfs
ISO_DIR      := build/iso
CHROOT_DIR   := build/chroot
DEVICE       ?=
QEMU_RAM     ?= 4096
QEMU_CPUS    ?= 4

all: check-paths

# ════════════════════════════════════════════════════════════════════════
# Docker-based build path (primary)
# ════════════════════════════════════════════════════════════════════════

# ── Build Docker image ───────────────────────────────────────────────
docker:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -f Dockerfile ..

# ── Extract rootfs from Docker image ─────────────────────────────────
rootfs: docker
	@echo "==> Extracting rootfs from Docker image..."
	rm -rf $(ROOTFS_DIR)
	mkdir -p $(ROOTFS_DIR)
	container_id=$$(docker create $(IMAGE_NAME):$(IMAGE_TAG)) && \
	docker export $$container_id | tar -C $(ROOTFS_DIR) -xf - && \
	docker rm $$container_id > /dev/null
	@echo "==> Rootfs extracted to $(ROOTFS_DIR)"

# ── Build bootable UEFI ISO (Docker path) ────────────────────────────
iso: rootfs
	@echo "==> Building bootable ISO..."
	mkdir -p $(ISO_DIR)/boot/grub $(ISO_DIR)/EFI/BOOT $(ISO_DIR)/live

	# Create squashfs of the rootfs
	mksquashfs $(ROOTFS_DIR) $(ISO_DIR)/live/filesystem.squashfs \
		-comp xz -Xbcj x86 -b 1M -no-duplicates -noappend

	# Copy kernel and initrd from rootfs
	cp $(ROOTFS_DIR)/boot/vmlinuz-* $(ISO_DIR)/live/vmlinuz
	cp $(ROOTFS_DIR)/boot/initrd.img-* $(ISO_DIR)/live/initrd.img

	# Install GRUB config
	cp grub/grub.cfg $(ISO_DIR)/boot/grub/grub.cfg

	# Copy EFI bootloader from rootfs
	@if [ -f $(ROOTFS_DIR)/usr/lib/shim/shimx64.efi.signed ]; then \
		cp $(ROOTFS_DIR)/usr/lib/shim/shimx64.efi.signed $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI; \
		cp $(ROOTFS_DIR)/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed $(ISO_DIR)/EFI/BOOT/grubx64.efi; \
	elif [ -f $(ROOTFS_DIR)/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi ]; then \
		cp $(ROOTFS_DIR)/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI; \
	fi

	# Build ISO with xorriso (UEFI)
	xorriso -as mkisofs \
		-o $(ISO_NAME) \
		-iso-level 3 \
		-full-iso9660-filenames \
		-volid "RALPHGLASSES" \
		-eltorito-alt-boot \
		-e EFI/BOOT/BOOTX64.EFI \
		-no-emul-boot \
		-isohybrid-gpt-basdat \
		-append_partition 2 0xef $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI \
		$(ISO_DIR)

	@echo ""
	@echo "==> ISO built: $(ISO_NAME)"
	@ls -lh $(ISO_NAME)

# ════════════════════════════════════════════════════════════════════════
# Debootstrap-based build path (alternative, no Docker required)
# ════════════════════════════════════════════════════════════════════════

# ── Install host dependencies for debootstrap path ───────────────────
install-deps:
	@echo "==> Installing build dependencies..."
	sudo apt-get update
	sudo apt-get install -y \
		debootstrap \
		squashfs-tools \
		xorriso \
		isolinux \
		syslinux-utils \
		grub-efi-amd64-bin \
		grub-pc-bin \
		mtools
	@echo "==> Dependencies installed"

# ── Create rootfs via debootstrap ────────────────────────────────────
rootfs-debootstrap: install-deps
	@echo "==> Creating rootfs via debootstrap..."
	@if [ -d $(CHROOT_DIR)/bin ]; then \
		echo "    Chroot already exists, skipping debootstrap (use 'make clean' to rebuild)"; \
	else \
		sudo debootstrap --arch=$(ARCH) noble $(CHROOT_DIR) http://archive.ubuntu.com/ubuntu; \
	fi
	@echo "==> Rootfs created at $(CHROOT_DIR)"

# ── Configure chroot environment ─────────────────────────────────────
chroot: rootfs-debootstrap
	@echo "==> Configuring chroot environment..."
	# Mount required filesystems
	sudo mount --bind /dev $(CHROOT_DIR)/dev 2>/dev/null || true
	sudo mount --bind /dev/pts $(CHROOT_DIR)/dev/pts 2>/dev/null || true
	sudo mount -t proc proc $(CHROOT_DIR)/proc 2>/dev/null || true
	sudo mount -t sysfs sysfs $(CHROOT_DIR)/sys 2>/dev/null || true

	# Copy DNS config
	sudo cp /etc/resolv.conf $(CHROOT_DIR)/etc/resolv.conf

	# Copy chroot setup script and distro configs
	sudo cp scripts/chroot-setup.sh $(CHROOT_DIR)/tmp/chroot-setup.sh
	sudo chmod 755 $(CHROOT_DIR)/tmp/chroot-setup.sh
	sudo mkdir -p $(CHROOT_DIR)/tmp/distro-configs
	sudo cp -r i3/ $(CHROOT_DIR)/tmp/distro-configs/i3/
	sudo cp -r systemd/ $(CHROOT_DIR)/tmp/distro-configs/systemd/
	sudo cp -r grub/ $(CHROOT_DIR)/tmp/distro-configs/grub/

	# Run setup inside chroot
	sudo chroot $(CHROOT_DIR) /bin/bash /tmp/chroot-setup.sh

	# Cleanup mounts
	sudo umount $(CHROOT_DIR)/sys 2>/dev/null || true
	sudo umount $(CHROOT_DIR)/proc 2>/dev/null || true
	sudo umount $(CHROOT_DIR)/dev/pts 2>/dev/null || true
	sudo umount $(CHROOT_DIR)/dev 2>/dev/null || true

	# Remove setup artifacts
	sudo rm -rf $(CHROOT_DIR)/tmp/chroot-setup.sh $(CHROOT_DIR)/tmp/distro-configs
	@echo "==> Chroot configured"

# ── Build bootable UEFI ISO (debootstrap path) ──────────────────────
iso-debootstrap: chroot
	@echo "==> Building ISO from debootstrap chroot..."
	mkdir -p $(ISO_DIR)/boot/grub $(ISO_DIR)/EFI/BOOT $(ISO_DIR)/live

	# Create squashfs of the chroot
	sudo mksquashfs $(CHROOT_DIR) $(ISO_DIR)/live/filesystem.squashfs \
		-comp xz -Xbcj x86 -b 1M -no-duplicates -noappend

	# Copy kernel and initrd from chroot
	sudo cp $(CHROOT_DIR)/boot/vmlinuz-* $(ISO_DIR)/live/vmlinuz
	sudo cp $(CHROOT_DIR)/boot/initrd.img-* $(ISO_DIR)/live/initrd.img

	# Install GRUB config
	cp grub/grub.cfg $(ISO_DIR)/boot/grub/grub.cfg

	# Copy EFI bootloader from chroot
	@if [ -f $(CHROOT_DIR)/usr/lib/shim/shimx64.efi.signed ]; then \
		sudo cp $(CHROOT_DIR)/usr/lib/shim/shimx64.efi.signed $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI; \
		sudo cp $(CHROOT_DIR)/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed $(ISO_DIR)/EFI/BOOT/grubx64.efi; \
	elif [ -f $(CHROOT_DIR)/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi ]; then \
		sudo cp $(CHROOT_DIR)/usr/lib/grub/x86_64-efi/monolithic/grubx64.efi $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI; \
	else \
		echo "==> No EFI bootloader found, generating with grub-mkimage..."; \
		grub-mkimage -o $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI -O x86_64-efi \
			part_gpt part_msdos fat iso9660 normal boot linux configfile \
			loopback chain efifwsetup efi_gop efi_uga ls search \
			search_label search_fs_uuid search_fs_file gfxterm gfxterm_background \
			gfxterm_menu test all_video loadenv exfat ext2 squash4; \
	fi

	# Build BIOS+UEFI hybrid ISO
	xorriso -as mkisofs \
		-o $(ISO_NAME) \
		-iso-level 3 \
		-full-iso9660-filenames \
		-volid "RALPHGLASSES" \
		-eltorito-alt-boot \
		-e EFI/BOOT/BOOTX64.EFI \
		-no-emul-boot \
		-isohybrid-gpt-basdat \
		-append_partition 2 0xef $(ISO_DIR)/EFI/BOOT/BOOTX64.EFI \
		$(ISO_DIR)

	@echo ""
	@echo "==> ISO built: $(ISO_NAME)"
	@ls -lh $(ISO_NAME)

# ════════════════════════════════════════════════════════════════════════
# Common targets
# ════════════════════════════════════════════════════════════════════════

# ── Validate built ISO ───────────────────────────────────────────────
validate:
	@echo "==> Validating ISO..."
	@if [ ! -f $(ISO_NAME) ]; then \
		echo "ERROR: $(ISO_NAME) not found. Run 'make iso' first."; \
		exit 1; \
	fi
	@file $(ISO_NAME) | grep -q "ISO 9660" && echo "    Type: Valid ISO 9660" || \
		(echo "    Type: INVALID — not an ISO 9660 image" && exit 1)
	@echo "    Size: $$(du -h $(ISO_NAME) | cut -f1)"
	@echo "    SHA256: $$(sha256sum $(ISO_NAME) | cut -d' ' -f1)"
	@echo "==> Validation passed"

# ── Validate grub.cfg (CI-safe, no ISO required) ─────────────────────
validate-grub:
	@bash scripts/validate-grub.sh grub/grub.cfg

# ── Test in QEMU (UEFI) ──────────────────────────────────────────────
test-vm:
	@if [ ! -f $(ISO_NAME) ]; then \
		echo "ERROR: $(ISO_NAME) not found. Run 'make iso' first."; \
		exit 1; \
	fi
	@echo "==> Launching QEMU with UEFI..."
	@echo "    RAM: $(QEMU_RAM)MB  CPUs: $(QEMU_CPUS)"
	@echo "    Press Ctrl+A X to quit"
	qemu-system-x86_64 \
		-enable-kvm \
		-m $(QEMU_RAM) \
		-smp $(QEMU_CPUS) \
		-bios /usr/share/OVMF/OVMF_CODE.fd \
		-cdrom $(ISO_NAME) \
		-boot d \
		-vga virtio \
		-display gtk \
		-net nic -net user

# ── Write ISO to USB drive ───────────────────────────────────────────
usb:
	@if [ ! -f $(ISO_NAME) ]; then \
		echo "ERROR: $(ISO_NAME) not found. Run 'make iso' first."; \
		exit 1; \
	fi
	@if [ -z "$(DEVICE)" ]; then \
		echo "ERROR: Specify target device, e.g.: make usb DEVICE=/dev/sdX"; \
		echo "Available devices:"; \
		lsblk -d -o NAME,SIZE,MODEL | grep -v loop; \
		exit 1; \
	fi
	@echo ""
	@echo "!!! WARNING: This will ERASE ALL DATA on $(DEVICE) !!!"
	@echo "    ISO: $(ISO_NAME)"
	@echo ""
	@read -p "Type YES to continue: " confirm && [ "$$confirm" = "YES" ] || exit 1
	sudo dd if=$(ISO_NAME) of=$(DEVICE) bs=4M status=progress oflag=sync
	sudo sync
	@echo "==> USB drive ready. Boot from $(DEVICE)."

# ── Check service file paths ────────────────────────────────────
# Validates all ExecStart= lines in *.service files use /usr/local/bin/.
# Scans all .service files under distro/ recursively.
# Run: make check-paths
check-paths:
	@bad=$$(find . -name '*.service' -type f -exec grep -Hn 'ExecStart=' {} + 2>/dev/null \
		| grep -v 'ExecStart=/usr/local/bin/'); \
	if [ -n "$$bad" ]; then \
		echo "ERROR: ExecStart paths must use /usr/local/bin/ prefix:"; \
		echo "$$bad"; \
		exit 1; \
	fi
	@echo "check-paths: OK"

# ── Clean build artifacts ────────────────────────────────────────────
clean:
	sudo rm -rf build/
	rm -f $(ISO_NAME)
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	@echo "==> Clean."
