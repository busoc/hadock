///////////////////////////////////////////////////////////
//  MVISPlugin.h
//  Implementation of the Class MVISPlugin
//  Created on:      17-Nov-2008 1:36:16 PM
//  Original author: Brijesh Saxena
///////////////////////////////////////////////////////////
#ifndef STREAMIZERPLUGIN_H_
#define STREAMIZERPLUGIN_H_

#include "VMUDPPlugin.h"

class StreamizerPlugin : public  VMUDPPlugin
{
public:
  StreamizerPlugin(struct Arguments* args);
  virtual ~StreamizerPlugin();
  virtual struct MulticastBufferStruct* HandleReceivedHrdlPacket(struct VMUPacketStruct const& vmuPacket);

private:
  int streams[3];
  static const int DefaultPort = 45000;
}

// The Class Factory Methods
// These two "C" Factory Functions manage the creation and destruction of the class StreamizerPlugin
extern "C" {
	extern VMUDPPlugin* Init(struct Arguments* args);
	extern void Reset(VMUDPPlugin* plugin);
}

#endif /*STREAMIZERPLUGIN_H_*/
