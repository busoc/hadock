///////////////////////////////////////////////////////////
//  MulticastBufferStruct.h
//  Implementation of the Class MulticastBufferStruct
//  Created on:      17-Nov-2008 1:36:15 PM
//  Original author: Brijesh Saxena
///////////////////////////////////////////////////////////

#ifndef MULTICASTBUFFERSTRUCT_H_
#define MULTICASTBUFFERSTRUCT_H_

#include "Buffer.h"

struct MulticastBufferStruct {
	
	std::string fileName;
	Buffer* imgBuffer;
	std::string xmlContent;
	unsigned int xmlContentLength;
	
};
#endif /*MULTICASTBUFFERSTRUCT_H_*/

