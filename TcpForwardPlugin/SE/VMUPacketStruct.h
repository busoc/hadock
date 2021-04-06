///////////////////////////////////////////////////////////
//  VMUPacketStruct.h
//  Implementation of the Class VMUPacketStruct
//  Created on:      04-Dec-2008 9:34:31 AM
//  Original author: Brijesh Saxena
///////////////////////////////////////////////////////////

#ifndef VMUPACKETSTRUCT_H_
#define VMUPACKETSTRUCT_H_

#include <iostream>

//using namespace dass;

struct VMUPacketStruct {
	
	unsigned char const* const pPacketBuffer;
	unsigned const int pPacketBufferLength;
    bool packetProblem;
    int syncPattern;
    int syncID;
    unsigned int userLength;
	
	VMUPacketStruct(unsigned char const* const buffer,
		unsigned const int bufferLength, bool packetProblem,
        int syncPattern, int syncID, unsigned int userLength);
	virtual ~VMUPacketStruct();
	
};

#endif /*VMUPACKETSTRUCT_H_*/

