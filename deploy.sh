#!/bin/bash
# deploy.sh - Deploy gassy to VM
# Usage: ./deploy.sh <vm-host> <ssh-key>
#
# This script deploys the latest gassy code to the VM by:
# 1. Creating a tarball of the repo
# 2. Copying it to the VM
# 3. Extracting and building everything
# 4. Starting the services

set -e

VM_HOST="${1:-cnovak@192.168.169.133}"
SSH_KEY="${2:-~/.ssh/id_ed25519}"

echo "=== Gassy Deployment ==="
echo "VM: $VM_HOST"

# Check for required tools locally
command -v ssh >/dev/null || { echo "ssh required but not installed"; exit 1; }
command -v scp >/dev/null || { echo "scp required but not installed"; exit 1; }

# Get the GitHub repo URL
REPO_URL=$(git remote get-url origin)
REPO_NAME=$(basename -s .git "$REPO_URL")

echo "Repo: $REPO_URL"

# Create tarball of current state (excluding large/unnecessary files)
echo "Creating tarball..."
TARBALL="/tmp/${REPO_NAME}.tar.gz"
git archive --prefix="$REPO_NAME/" HEAD | gzip > "$TARBALL"

echo "Copying to VM..."
scp -i "$SSH_KEY" "$TARBALL" "$VM_HOST:/tmp/

echo "Extracting on VM..."
ssh -i "$SSH_KEY" "$VM_HOST" "
    set -e
    cd /tmp
    rm -rf ${REPO_NAME}
    tar -xzf ${REPO_NAME}.tar.gz
    rm -rf /home/cnovak/gassy.old
    mv /home/cnovak/gassy /home/cnovak/gassy.old
    mv /tmp/${REPO_NAME} /home/cnovak/gassy
    cd /home/cnovak/gassy

    # Set up Go environment
    export PATH=/home/cnovak/go/bin:\$PATH
    export GOROOT=/home/cnovak/go
    export GOPATH=/home/cnovak/gopath
    mkdir -p \$GOPATH

    # Restore .env if it exists in old directory
    if [ -f /home/cnovak/gassy.old/.env ]; then
        cp /home/cnovak/gassy.old/.env .
    fi

    # Build and install everything
    echo 'Building agent image...'
    make build-agent

    echo 'Installing CLI binaries...'
    make install

    echo 'Deployment complete!'
    echo ''
    echo 'To start services: gassy-admin start'
"

echo ""
echo "=== Deployment Complete ==="
echo "SSH to VM and run: gassy-admin start"
