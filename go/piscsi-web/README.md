# PiSCSI Web Interface (Go)

Go implementation of the PiSCSI web interface, providing a browser-based control panel for the PiSCSI SCSI emulator.

## Components

### Web Interface (`piscsi-web`)
HTTP server providing:
- Session-based authentication (PAM or fallback)
- Device management (attach/detach SCSI devices)
- File operations (upload/download/manage disk images)
- System administration
- Dual-mode API (HTML forms + JSON)

**Architecture:**
- **Form-post centric**: POST-Redirect-GET pattern with flash messages
- **Minimal JavaScript**: Designed for retro computers (only confirmations/prompts)
- **Template-driven**: Server-side HTML rendering via Go templates

### Mock PiSCSI Daemon (`mock-piscsi`)
Minimal gRPC server simulating the PiSCSI daemon for development/testing without hardware.

## Building

### Requirements
- Go 1.21+
- Protocol Buffers compiler (for gRPC)

Or use the provided Nix shell:
```bash
nix-shell
```

### Build Commands

**Web interface:**
```bash
go build -o piscsi-web ./cmd/piscsi-web
```

**Mock daemon:**
```bash
go build -o mock-piscsi ./cmd/mock-piscsi
```


**Regenerate protobuf bindings** (only needed after proto changes):
```bash
make proto
```

## Running

### Development Mode
```bash
# Terminal 1: Start mock daemon
./mock-piscsi

# Terminal 2: Start web interface
DISABLE_PAM=true FALLBACK_USER=admin FALLBACK_PASSWORD=admin ./piscsi-web
```

Access at: http://localhost:8080

### Production Mode
```bash
./piscsi-web
```

Default configuration:
- Server: `0.0.0.0:8080`
- PiSCSI daemon: `localhost:6868`
- Authentication: PAM (group: `piscsi`)

## Configuration

Environment variables:
- `DISABLE_PAM`: Use fallback auth instead of PAM
- `FALLBACK_USER` / `FALLBACK_PASSWORD`: Credentials for fallback auth
- `PISCSI_HOST`: PiSCSI daemon host (default: `localhost`)
- `PISCSI_PORT`: PiSCSI daemon port (default: `6868`)
- `SERVER_PORT`: Web server port (default: `8080`)

## Project Structure

```
.
├── cmd/
│   ├── piscsi-web/     # Web server entrypoint
│   └── mock-piscsi/    # Mock daemon entrypoint
├── internal/
│   ├── server/         # HTTP handlers, routing, flash messages
│   ├── piscsi/         # gRPC client for PiSCSI daemon
│   ├── auth/           # Authentication (PAM + fallback)
│   ├── config/         # Configuration management
│   └── driveprops/     # Drive properties database
├── web/
│   ├── templates/      # HTML templates
│   └── static/         # CSS, JS, images
├── drive_properties.json
└── mock-piscsi         # Built mock daemon
```

## Development Notes

- Templates are embedded in the binary via `embed` directives
- Session storage uses gorilla/sessions with cookie-based persistence
- Flash messages require `gob.Register()` for serialization
- Proto definitions in `proto/piscsi_interface.proto` should match `/cpp/piscsi_interface.proto`
- Regenerate Go bindings with `make proto` after updating the proto file
- The generated `piscsi_interface.pb.go` file is committed to the repository
