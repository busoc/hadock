//==============================================================================
// TcpForwardPlugin.cpp
//==============================================================================
//
// Author: Didier Pieroux (BIRA-IASB)
//
// See TcpForwardPlugin.txt for an overview of the implementation.
//==============================================================================

#include <arpa/inet.h>
#include <stdexcept>
#include <climits>
#include <cstring>

#include "Logging.h"

#include "ArgumentHandler.h"
#include "TcpForwardTools.h"
#include "TcpForwardPlugin.h"
#include "TcpForwardClient.h"
#include "VMUPacketStruct.h"

using namespace std;

//==============================================================================
// Static variables
//==============================================================================

static int missionMode;
static uint16_t sequenceCounter = 0;
static pthread_mutex_t * const sequenceCounterMutex = new_pthread_mutex_t();


//==============================================================================
// class TcpForwardPlugin
//==============================================================================

TcpForwardPlugin::TcpForwardPlugin(Arguments* args)
    : VMUDPPlugin(args)
{}

//------------------------------------------------------------------------------

void TcpForwardPlugin::handlePacket(const unsigned char *packetData, 
                                    int packetSize)
{
    if (packetSize == 0) return;

    if (packetData == 0)
    {
        logError("TcpForwardPlugin: got empty VMU buffer pointer");
        return;
    }

    uint16_t sequence;
    {
        ThreadLocker locker (sequenceCounterMutex);
        sequence = sequenceCounter++;
    }


    int const bufSize = packetSize+14; // 12 header bytes and 2 checksum bytes
    unsigned char * const buf = new unsigned char[bufSize];

    if (!buf)
    {
        logError("TcpForwardPlugin: failure to allocate %d bytes", bufSize);
        return;
    }

    buf[0] = 0xf8; // 4-byte header
    buf[1] = 0x2e;
    buf[2] = 0x35;
    buf[3] = 0x53;
    buf[4] = 0x02; // Protocol: 0, VMU Version: MkII
    buf[5] = missionMode;
    *(uint16_t *)(buf+6) = htons(sequence);
    *(uint32_t *)(buf+8) = htonl(packetSize);
    memcpy(buf+12, packetData, packetSize);
    *(uint16_t *)(buf+packetSize+12) = ip_checksum(buf, bufSize-2);

    TcpForwardClient::send(buf);
}

//------------------------------------------------------------------------------

MulticastBufferStruct*
TcpForwardPlugin::HandleReceivedHrdlPacket(const VMUPacketStruct & vmuPacket)
{
    handlePacket(vmuPacket.pPacketBuffer, vmuPacket.pPacketBufferLength);
    return 0;
}

//------------------------------------------------------------------------------

extern "C" VMUDPPlugin* Init(struct Arguments* args)
{
    logInfo("TcpForwardPlugin: Init called");
    logInfo("TcpForwardPlugin: parameters: %s", args->pluginParams.c_str());

    istringstream is(args->pluginParams);
    string garbage;

    string host;
    int port = -1;
    int reconnectDelay = -1;
    unsigned int queueSize = 0;

    getline(is, host, ':');    is >> port;
    getline(is, garbage, ','); is >> reconnectDelay;
    getline(is, garbage, ','); is >> queueSize;

    if (port == -1 || reconnectDelay == -1 || reconnectDelay == -99)
    {
        logError("TcpForwardPlugin: wrong parameter format");
        logError("TcpForwardPlugin: format: "
                 "-pp \"host:port, reconnectTime, queueSize\"");
        return 0;
    }

    logInfo("TcpForwardPlugin: server host: %s; port: %d", host.c_str(), port);
    logInfo("TcpForwardPlugin: reconnect delay: %d s", reconnectDelay);
    if (queueSize>0)
    {
        logInfo("TcpForwardPlugin: msg queue size: %d", queueSize);
    }
    else
    {
        logInfo("TcpForwardPlugin: unlimited msg queue size");
        queueSize=0;
    }


    missionMode = args->MissionMode;
    logInfo("TcpForwardPlugin: mission mode: %d", missionMode);

    try
    {
        TcpForwardClient::init(host, port, reconnectDelay, queueSize);
        TcpForwardClient::start();
        return new TcpForwardPlugin(args);
    }
    catch(runtime_error&)
    {
        TcpForwardClient::shutdown();
        return 0;
    }
}

//------------------------------------------------------------------------------

extern "C" void Reset(VMUDPPlugin* plugin)
{
    logInfo("TcpForwardPlugin: Reset called");

    TcpForwardClient::shutdown();

    delete plugin;
}

//==============================================================================
