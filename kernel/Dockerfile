FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    build-essential bc flex bison libelf-dev libssl-dev \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Clone the Amazon Linux kernel (same source Firecracker uses)
RUN git clone --no-checkout --filter=tree:0 https://github.com/amazonlinux/linux.git

WORKDIR /build/linux

# Find and checkout the latest microvm kernel tag for 6.1
RUN TAG=$(git tag -l --sort=-creatordate | grep "microvm-kernel-6.1\..*\.amzn2" | head -n1) && \
    echo "Using kernel tag: $TAG" && \
    git checkout $TAG

# Copy Firecracker base config + FCM overrides
COPY base.config .config
COPY fcm-overrides.config /tmp/fcm-overrides.config

# Apply overrides
RUN while IFS= read -r line; do \
        [ -z "$line" ] && continue; \
        [[ "$line" =~ ^# ]] && continue; \
        key="${line%%=*}"; \
        value="${line#*=}"; \
        sed -i "s/^# ${key} is not set/${key}=${value}/" .config; \
        grep -q "^${key}=" .config || echo "${key}=${value}" >> .config; \
    done < /tmp/fcm-overrides.config

# Resolve dependencies
RUN make olddefconfig

# Build
RUN make -j$(nproc) vmlinux

# Verify
RUN grep "CONFIG_VFAT_FS=y" .config && \
    grep "CONFIG_FAT_FS=y" .config && \
    grep "CONFIG_ISO9660_FS=y" .config && \
    echo "=== FCM kernel build verified ==="
