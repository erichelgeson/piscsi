//---------------------------------------------------------------------------
//
// SCSI Target Emulator PiSCSI
// for Raspberry Pi
//
// Copyright (C) 2022 Uwe Seimet
//
//---------------------------------------------------------------------------

#include <gtest/gtest.h>

#include "shared/piscsi_exceptions.h"

using namespace scsi_defs;

TEST(PiscsiExceptionsTest, IoException)
{
	try {
		throw io_exception("msg");
	}
	catch(const io_exception& e) {
		EXPECT_STREQ("msg", e.what());
	}
}

TEST(PiscsiExceptionsTest, FileNotFoundException)
{
	try {
		throw file_not_found_exception("msg");
	}
	catch(const file_not_found_exception& e) {
		EXPECT_STREQ("msg", e.what());
	}
}

TEST(PiscsiExceptionsTest, ScsiErrorException)
{
	try {
		throw scsi_exception(sense_key::UNIT_ATTENTION);
	}
	catch(const scsi_exception& e) {
		EXPECT_EQ(sense_key::UNIT_ATTENTION, e.get_sense_key());
		EXPECT_EQ(asc::NO_ADDITIONAL_SENSE_INFORMATION, e.get_asc());
	}

	try {
		throw scsi_exception(sense_key::UNIT_ATTENTION, asc::LBA_OUT_OF_RANGE);
	}
	catch(const scsi_exception& e) {
		EXPECT_EQ(sense_key::UNIT_ATTENTION, e.get_sense_key());
		EXPECT_EQ(asc::LBA_OUT_OF_RANGE, e.get_asc());
	}
}
