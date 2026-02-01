#!/bin/bash
#
# RAGFlow NAS Paperless Mount Helper Script
# 
# This script helps you mount a NAS share containing Paperless documents
# and configure RAGFlow to access them.
#
# Usage: ./mount_nas_paperless.sh

set -e

echo "================================================"
echo "RAGFlow NAS Paperless Document Mount Helper"
echo "================================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored messages
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running with sudo/root
if [ "$EUID" -ne 0 ]; then 
    print_error "This script requires root privileges for mounting NAS storage."
    echo "Please run with sudo: sudo $0"
    exit 1
fi

# Get mount type
echo "Select your NAS protocol:"
echo "1) NFS"
echo "2) SMB/CIFS"
read -p "Enter choice [1-2]: " PROTOCOL_CHOICE

if [ "$PROTOCOL_CHOICE" = "1" ]; then
    PROTOCOL="nfs"
elif [ "$PROTOCOL_CHOICE" = "2" ]; then
    PROTOCOL="cifs"
else
    print_error "Invalid choice. Exiting."
    exit 1
fi

# Get NAS details
read -p "Enter NAS IP address: " NAS_IP
read -p "Enter NAS share path (e.g., /volume1/paperless or paperless): " NAS_SHARE
read -p "Enter local mount point [/mnt/nas-paperless]: " MOUNT_POINT
MOUNT_POINT=${MOUNT_POINT:-/mnt/nas-paperless}

# Create mount point
print_info "Creating mount point: $MOUNT_POINT"
mkdir -p "$MOUNT_POINT"

# Mount based on protocol
if [ "$PROTOCOL" = "nfs" ]; then
    print_info "Mounting NFS share..."
    
    # Check if NFS client is installed
    if ! command -v mount.nfs &> /dev/null; then
        print_warn "NFS client not found. Installing..."
        if command -v apt-get &> /dev/null; then
            apt-get update && apt-get install -y nfs-common
        elif command -v yum &> /dev/null; then
            yum install -y nfs-utils
        else
            print_error "Could not install NFS client. Please install manually."
            exit 1
        fi
    fi
    
    # Test NFS connection
    print_info "Testing NFS connection..."
    if showmount -e "$NAS_IP" &> /dev/null; then
        print_info "NFS server is reachable"
    else
        print_warn "Cannot connect to NFS server. Proceeding anyway..."
    fi
    
    # Mount NFS
    mount -t nfs "${NAS_IP}:${NAS_SHARE}" "$MOUNT_POINT"
    FSTAB_ENTRY="${NAS_IP}:${NAS_SHARE}  $MOUNT_POINT  nfs  defaults,_netdev  0  0"
    
elif [ "$PROTOCOL" = "cifs" ]; then
    print_info "Mounting SMB/CIFS share..."
    
    # Check if CIFS client is installed
    if ! command -v mount.cifs &> /dev/null; then
        print_warn "CIFS client not found. Installing..."
        if command -v apt-get &> /dev/null; then
            apt-get update && apt-get install -y cifs-utils
        elif command -v yum &> /dev/null; then
            yum install -y cifs-utils
        else
            print_error "Could not install CIFS client. Please install manually."
            exit 1
        fi
    fi
    
    # Get credentials
    read -p "Enter username: " SMB_USER
    read -sp "Enter password: " SMB_PASS
    echo ""
    
    # Create credentials file
    CREDS_FILE="/root/.smbcredentials_ragflow"
    print_info "Creating credentials file: $CREDS_FILE"
    cat > "$CREDS_FILE" << EOF
username=$SMB_USER
password=$SMB_PASS
EOF
    chmod 600 "$CREDS_FILE"
    
    # Mount CIFS
    mount -t cifs "//${NAS_IP}/${NAS_SHARE}" "$MOUNT_POINT" -o "credentials=$CREDS_FILE"
    FSTAB_ENTRY="//${NAS_IP}/${NAS_SHARE}  $MOUNT_POINT  cifs  credentials=$CREDS_FILE,_netdev  0  0"
fi

# Verify mount
if mountpoint -q "$MOUNT_POINT"; then
    print_info "Successfully mounted NAS at $MOUNT_POINT"
    ls -la "$MOUNT_POINT" | head -10
else
    print_error "Failed to mount NAS"
    exit 1
fi

# Ask about persistent mount
echo ""
read -p "Would you like to make this mount persistent (add to /etc/fstab)? [y/N]: " PERSIST
if [[ "$PERSIST" =~ ^[Yy]$ ]]; then
    # Check if entry already exists
    if grep -q "$MOUNT_POINT" /etc/fstab; then
        print_warn "An entry for $MOUNT_POINT already exists in /etc/fstab"
    else
        print_info "Adding entry to /etc/fstab..."
        echo "$FSTAB_ENTRY" >> /etc/fstab
        print_info "Entry added to /etc/fstab"
    fi
fi

# Docker Compose configuration
echo ""
echo "================================================"
echo "Docker Compose Configuration"
echo "================================================"
print_info "Add the following to your docker-compose.yml under the ragflow service volumes section:"
echo ""
echo "    volumes:"
echo "      # ... existing volumes ..."
echo "      - ${MOUNT_POINT}:/ragflow/paperless:ro"
echo ""

# Ask if they want to update docker-compose.yml automatically
read -p "Would you like to automatically add this volume to docker-compose.yml? [y/N]: " AUTO_UPDATE
if [[ "$AUTO_UPDATE" =~ ^[Yy]$ ]]; then
    COMPOSE_FILE="docker-compose.yml"
    
    if [ ! -f "$COMPOSE_FILE" ]; then
        print_error "docker-compose.yml not found in current directory"
        print_info "Please add the volume manually"
    else
        # Create backup
        cp "$COMPOSE_FILE" "${COMPOSE_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
        print_info "Created backup: ${COMPOSE_FILE}.backup"
        
        # Add volume mount (simple append after first volumes: section)
        # This is a simple implementation - for production, use a proper YAML parser
        print_warn "Automatic update is experimental. Please verify the changes."
        sed -i "/ragflow-cpu:/,/^[^ ]/ s|volumes:|volumes:\n      - ${MOUNT_POINT}:/ragflow/paperless:ro  # NAS Paperless mount|" "$COMPOSE_FILE"
        
        print_info "Updated $COMPOSE_FILE (backup created)"
        print_warn "Please review the changes before restarting RAGFlow"
    fi
fi

echo ""
echo "================================================"
echo "Next Steps"
echo "================================================"
echo "1. Review your docker-compose.yml file"
echo "2. Restart RAGFlow:"
echo "   cd docker"
echo "   docker compose down"
echo "   docker compose up -d"
echo "3. Verify access from container:"
echo "   docker exec -it ragflow-cpu ls -la /ragflow/paperless"
echo "4. Access documents in RAGFlow UI from /ragflow/paperless directory"
echo ""
print_info "Setup complete!"
