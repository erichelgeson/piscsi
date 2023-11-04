//---------------------------------------------------------------------------
//
// SCSI Target Emulator PiSCSI
// for Raspberry Pi
//
// Copyright (C) 2022-2023 Uwe Seimet
//
//---------------------------------------------------------------------------

#pragma once

#include "hal/bus.h"
#include "scsidump/scsi_executor.h"
#include <string>
#include <span>
#include <vector>
#include <unordered_map>
#include <iostream>

using namespace std;

class ScsiDump
{

public:

    ScsiDump() = default;
    ~ScsiDump() = default;

    int run(span<char *>);

    struct inquiry_info {
        string vendor;
        string product;
        string revision;
        uint32_t sector_size;
        uint64_t capacity;

        void GeneratePropertiesFile(ostream&, const string&) const;
    };
    using inquiry_info_t = struct inquiry_info;

private:

    bool Banner(ostream&, span<char *>) const;
    bool Init();
    void ParseArguments(span<char *>);
    void DisplayBoardId(ostream&) const;
    long CalculateEffectiveSize(ostream&) const;
    void ScanBus(ostream&);
    bool DisplayInquiry(ostream&, bool);
    string DumpRestore(ostream&);
    bool GetDeviceInfo(ostream&);

    void Reset() const;

    void CleanUp() const;
    static void TerminationHandler(int);

    unique_ptr<BUS> bus;

    unique_ptr<ScsiExecutor> scsi_executor;

    inquiry_info_t inq_info;

    vector<uint8_t> buffer;

    int initiator_id = 7;
    int target_id = -1;
    int target_lun = 0;

    string filename;

    bool to_stdout = false;

    bool run_inquiry = false;

    bool run_bus_scan = false;

    bool scan_all_luns = false;

    bool restore = false;

    bool create_properties_file = false;

    // Required for the termination handler
	static inline ScsiDump *instance;

	static const int MINIMUM_BUFFER_SIZE = 1024 * 64;
    static const int DEFAULT_BUFFER_SIZE = 1024 * 1024;

    static inline const string DIVIDER = "----------------------------------------";

    static inline const unordered_map<byte, string> DEVICE_TYPES = {
    		{ byte{0}, "Direct Access" },
			{ byte{1}, "Sequential Access" },
			{ byte{2}, "Printer" },
			{ byte{3}, "Processor" },
			{ byte{4}, "Write-Once" },
			{ byte{5}, "CD-ROM/DVD/BD/DVD-RAM" },
			{ byte{6}, "Scanner" },
			{ byte{7}, "Optical Memory" },
			{ byte{8}, "Media Changer" },
			{ byte{9}, "Communications" },
			{ byte{10}, "Graphic Arts Pre-Press" },
			{ byte{11}, "Graphic Arts Pre-Press" },
			{ byte{12}, "Storage Array Controller" },
			{ byte{13}, "Enclosure Services" },
			{ byte{14}, "Simplified Direct Access" },
			{ byte{15}, "Optical Card Reader/Writer" },
			{ byte{16}, "Bridge Controller" },
			{ byte{17}, "Object-based Storage" },
			{ byte{18}, "Automation/Drive Interface" },
			{ byte{19}, "Security Manager" },
			{ byte{20}, "Host Managed Zoned Block" },
			{ byte{30}, "Well Known Logical Unit" }
    };
};
