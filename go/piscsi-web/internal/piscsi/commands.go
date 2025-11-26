package piscsi

import (
	pb "github.com/piscsi/piscsi-web/proto"
)

// CommandBuilder provides methods to build protobuf commands for the piscsi daemon
type CommandBuilder struct {
	token  string
	locale string
}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		locale: "en",
	}
}

// SetToken sets the authentication token for commands
func (cb *CommandBuilder) SetToken(token string) {
	cb.token = token
}

// SetLocale sets the locale for error messages
func (cb *CommandBuilder) SetLocale(locale string) {
	cb.locale = locale
}

// ServerInfo creates a command to get server information
func (cb *CommandBuilder) ServerInfo() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_SERVER_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// ListDevices creates a command to list all attached devices
func (cb *CommandBuilder) ListDevices() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_DEVICES_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// ListImages creates a command to list available image files
func (cb *CommandBuilder) ListImages(defaultFolder string) *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_DEFAULT_IMAGE_FILES_INFO,
		Params: map[string]string{
			"folder": defaultFolder,
		},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// AttachDevice creates a command to attach a device
func (cb *CommandBuilder) AttachDevice(scsiID, lun int32, deviceType pb.PbDeviceType, file string, params map[string]string) *pb.PbCommand {
	device := &pb.PbDeviceDefinition{
		Id:   scsiID,
		Unit: lun,
		Type: deviceType,
	}

	// Add vendor/product/revision if provided
	if vendor, ok := params["vendor"]; ok {
		device.Vendor = vendor
	}
	if product, ok := params["product"]; ok {
		device.Product = product
	}
	if revision, ok := params["revision"]; ok {
		device.Revision = revision
	}

	// Add image file if provided
	if file != "" {
		device.Params = map[string]string{
			"file": file,
		}
	}

	// Add any additional parameters
	if device.Params == nil {
		device.Params = make(map[string]string)
	}
	for k, v := range params {
		if k != "vendor" && k != "product" && k != "revision" {
			device.Params[k] = v
		}
	}

	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_ATTACH,
		Devices:   []*pb.PbDeviceDefinition{device},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// DetachDevice creates a command to detach a specific device
func (cb *CommandBuilder) DetachDevice(scsiID, lun int32) *pb.PbCommand {
	device := &pb.PbDeviceDefinition{
		Id:   scsiID,
		Unit: lun,
	}

	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_DETACH,
		Devices:   []*pb.PbDeviceDefinition{device},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// DetachAll creates a command to detach all devices
func (cb *CommandBuilder) DetachAll() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_DETACH_ALL,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// EjectDevice creates a command to eject removable media
func (cb *CommandBuilder) EjectDevice(scsiID, lun int32) *pb.PbCommand {
	device := &pb.PbDeviceDefinition{
		Id:   scsiID,
		Unit: lun,
	}

	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_EJECT,
		Devices:   []*pb.PbDeviceDefinition{device},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// ReserveIDs creates a command to reserve SCSI IDs
func (cb *CommandBuilder) ReserveIDs(ids []int32) *pb.PbCommand {
	idsStr := ""
	for i, id := range ids {
		if i > 0 {
			idsStr += ","
		}
		idsStr += string(rune(id + '0'))
	}

	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_RESERVE_IDS,
		Params: map[string]string{
			"ids": idsStr,
		},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// SaveConfiguration creates a command to save current configuration to a file
func (cb *CommandBuilder) SaveConfiguration(filename string) *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_NO_OPERATION,
		Params: map[string]string{
			"file": filename,
		},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// GetVersion creates a command to get version information
func (cb *CommandBuilder) GetVersion() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_VERSION_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// GetLogLevel creates a command to get current log level
func (cb *CommandBuilder) GetLogLevel() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_LOG_LEVEL_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// SetLogLevel creates a command to set log level
func (cb *CommandBuilder) SetLogLevel(level string) *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_LOG_LEVEL,
		Params: map[string]string{
			"level": level,
		},
	}
	cb.setCommonParams(cmd)
	return cmd
}

// GetNetworkInfo creates a command to get network interface information
func (cb *CommandBuilder) GetNetworkInfo() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_NETWORK_INTERFACES_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// GetDeviceTypesInfo creates a command to get information about supported device types
func (cb *CommandBuilder) GetDeviceTypesInfo() *pb.PbCommand {
	cmd := &pb.PbCommand{
		Operation: pb.PbOperation_DEVICE_TYPES_INFO,
	}
	cb.setCommonParams(cmd)
	return cmd
}

// setCommonParams sets common parameters for all commands
func (cb *CommandBuilder) setCommonParams(cmd *pb.PbCommand) {
	if cb.token != "" {
		if cmd.Params == nil {
			cmd.Params = make(map[string]string)
		}
		cmd.Params["token"] = cb.token
	}

	if cb.locale != "" {
		if cmd.Params == nil {
			cmd.Params = make(map[string]string)
		}
		cmd.Params["locale"] = cb.locale
	}
}
