///////////////////////////////////////////////////////////
//  VMUDPPlugin.h
//  Implementation of the Class VMUDPPlugin
//  Created on:      17-Nov-2008 1:36:20 PM
//  Original author: Brijesh Saxena
///////////////////////////////////////////////////////////

#ifndef VMUDPPLUGIN_H_
#define VMUDPPLUGIN_H_

#include <string>
#include <fstream>
#include <stdio.h>
#include <time.h>
#include <sstream>
#include <stdlib.h>
#include <iostream>
#include <sys/time.h>

//#include <CCSDSPacket.h>
#include <hrdl/HRDLPacket.h>
#include <ServiceDescriptor.h>
#include <HRDLClientConnection.h>
#include <RealtimeHRDLSpecification.h>

#include "ImageMetaXml.h"
#include "ArgumentHandler.h"
#include "Buffer.h"
#include "VMUPacketStruct.h"

extern "C" { 
	#include "Logging.h"
}

#include "MulticastBufferStruct.h"

class VMUDPPlugin {

public:
	
	enum headerType { VHDPH, VIPH, SDPH, MVIS };
	enum fileStatusType { FILE_NOT_READY=0, FILE_READY=1, FILE_DONE=2, FILE_BAD=-1 };
	enum LogLevel {	LEVEL_INFO, LEVEL_ERROR };
	
	VMUDPPlugin(struct Arguments* args);
	virtual ~VMUDPPlugin();
	static char const* const SAVE_FILENAME_FORMAT()
	{ 
		return "%s_%013llu%s";
        }
	unsigned long getLong(const unsigned char* b, VMUDPPlugin::headerType header,
		int startIndex, int length);
	/* Not in use. No code written.
	unsigned long getLongLSB(const unsigned char* b, VMUDPPlugin::headerType header,
		int startIndex, int length);
	*/
	char* longToString(long n);
	//void log(char* b, const enum LogLevel level = VMUDPPlugin::LEVEL_ERROR);
	unsigned long getHCI(struct VMUPacketStruct const& vmuPacket);
	unsigned long getSID(struct VMUPacketStruct const& vmuPacket);
	unsigned long getSPARE1(struct VMUPacketStruct const& vmuPacket, 
		VMUDPPlugin::headerType header);
	unsigned long getSPARE2(struct VMUPacketStruct const& vmuPacket, 
		VMUDPPlugin::headerType header);
	
	unsigned long getSEQ(struct VMUPacketStruct const& vmuPacket, 
		VMUDPPlugin::headerType header);
	
	unsigned long long getTIM(struct VMUPacketStruct const& vmuPacket, 
		VMUDPPlugin::headerType header);
	int writeToFile(std::string folder, char* fileName, const char* buffer,
		int length);
	/**
	* Original Source: IncomingVmuHandler
	*/
	static char const* const TIME_FORMAT()
	{
	  return "%Y.%m.%d.%H.%M.%S";
        }
	char* createMetaXmlFileName(char const* const fileName);
	char* doubleToString(double d);
	static long getGpsToUnixEpoch();
	std::string createMetaXml(std::string filename, std::string metaFilename,
	char * const time, std::string s_HCI, std::string s_SID, std::string s_SEQ, 
	std::string s_TIM, std::string s_SEQviph, std::string s_TIMviph,
	std::string s_POR, std::string s_VID, std::string s_TYP, std::string s_FRT, 
	std::string s_PIXpx, std::string s_PIXpy, std::string s_ROIxof, 
	std::string s_ROIxsz, std::string s_ROIyof, std::string s_ROIysz, 
	std::string s_DRPldrp,	std::string s_DRPfdrp,	std::string s_UPI);
	
	static std::string getMissionModeText(int const mm);
	
	std::string createMetaXmlForScienceDataFile(std::string filename,
	std::string onboardFilename, std::string metaFilename, char * const time, 
	std::string s_HCI, std::string s_SID, std::string s_SEQ, std::string s_TIM, 
	std::string s_SEQsdph);

	virtual struct MulticastBufferStruct* HandleReceivedHrdlPacket(
		struct VMUPacketStruct const& vmuPacket) = 0;

	static const int NO_FILENAME_ID_DIGITS = 4;
	
	/**
	 * 1980.01.06.00.00.00
	 */
	static const unsigned long BEGINNING_OF_TIME = 315964800;
	
	static const unsigned int MAX_FILENAME_SIZE = 255;
	/**
	 * Original Source: IncomingVmuHandler
	 */
	static const unsigned int TIME_FORMAT_SIZE = 20;
	
protected:
	
	const int VIPH_OFFSET;
	const int SDPH_OFFSET;
	const int VHDPH_OFFSET;
	
	/**
	 * Original Source: IncomingVmuHandler
	 */
	struct Arguments* lArgs;
	/**
	 * Original Source: IncomingVmuHandler
	 */
	unsigned int outStandingPackets;
	/**
	 * Original Source: IncomingVmuHandler
	 */
	std::string saveFolder;
	
private:

	unsigned int iDataType;
	
	/**
	 * Original Source: IncomingVmuHandler
	 */
	ImageMetaXml lImageMetaXml;
	
	std::string getSeqIdString(long id);
	
};

/*
* The Types of the Class Factories
*/
typedef VMUDPPlugin* create_t(struct Arguments* args);
typedef void destroy_t(VMUDPPlugin* plugin);

#endif /*VMUDPPLUGIN_H_*/
