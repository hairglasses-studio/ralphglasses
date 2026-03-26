# ralphglasses bootable ISO build system
# Usage:
#   make iso              Build bootable UEFI ISO
#   make docker           Build Docker image only (for testing)
#   make test-vm          Launch ISO in QEMU
#   make usb DEVICE=X     Write ISO to USB drive
#   make clean            Remove build artifacts

IMAGE_NAME   := ralphglasses-os
IMAGE_TAG    := latest
ISO_NAME     := ralphglasses.iso
ROOTFS_DIR   := build/rootfs
ISO_DIR      := build/iso
DEVICE       ?=
QEMU_RAM     ?= 4096
QEMU_CPUS    ?= 4

.PHONY: all iso docker test-vm usb clean rootfs check-paths

all: check-paths

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

# ── Build bootable UEFI ISO ──────────────────────────────────────────
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

# ── Test in QEMU (UEFI) ──────────────────────────────────────────────
test-vm: iso
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
usb: iso
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
	rm -rf build/ $(ISO_NAME)
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	@echo "==> Clean."
