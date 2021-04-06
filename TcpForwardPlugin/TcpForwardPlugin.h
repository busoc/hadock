//==============================================================================
// TcpForwardPlugin.h
//==============================================================================
//
// Author: Didier Pieroux (BIRA-IASB)
//
// Upon reception of a VMU packet, this plug-in splits the packet into a
// sequence of small fragments and sends them all to a TCP server.
//
// See TcpForwardPlugin.txt for an overview of the implementation.
//==============================================================================

#ifndef TCP_FORWARD_PLUGIN_H
#define TCP_FORWARD_PLUGIN_H

class Arguments;
class VMUPacketStruct;
class MulticastBufferStruct;

#ifdef STANDALONE_BUILD
#warning VMUDDPlugin compiled in standalone mode
class VMUDPPlugin
{
public:
    VMUDPPlugin(Arguments* args) {};
    virtual ~VMUDPPlugin() {};

protected:
    struct Arguments* lArgs;
};
#else
#include "VMUDPPlugin.h"
#endif


//==============================================================================
// TcpForwardPlugin
//
// See the documentation of the stream extractor for an explanation about the
// interface.
//==============================================================================

class TcpForwardPlugin : public VMUDPPlugin
{
public:
	TcpForwardPlugin(Arguments *args);

    virtual ~TcpForwardPlugin() {}

    virtual MulticastBufferStruct* HandleReceivedHrdlPacket(
        const VMUPacketStruct &vmuPacket);

    static void handlePacket(const unsigned char *packetData, int packetSize);
};

//==============================================================================

extern "C"
{
    extern VMUDPPlugin* Init(Arguments* args);
    extern void Reset(VMUDPPlugin* plugin);
}

//==============================================================================

#endif // TCP_FORWARD_PLUGIN_H
