//==============================================================================
// TcpForwardClient.cpp
//==============================================================================
//
// Author: Didier Pieroux (BIRA-IASB)
//
// See TcpForwardPlugin.txt for an overview of the implementation.
//==============================================================================

#include <arpa/inet.h>
#include <cstring>
#include <ctime>
#include <errno.h>
#include <netdb.h>
#include <queue>
#include <sstream>
#include <stdexcept>
#include <sys/socket.h>
#include <unistd.h>

#include "ArgumentHandler.h"
#include "Logging.h"
#include "TcpForwardTools.h"
#include "TcpForwardClient.h"

using namespace std;


//==============================================================================
// Static variables and functions
//==============================================================================

static int const sleepDuration = 5000; //5 ms

static pthread_mutex_t *bufferQueueMutex = new_pthread_mutex_t();
static queue<unsigned char *> bufferQueue;
static unsigned int queueSize;

static volatile bool active = false;
static volatile bool connected = false;

static string host = "";
static int port = -1;
static int reconnectDelay = -1;
static int socket_id = -1;
static time_t lastConnectAttempt;

static pthread_t *sendThread = new pthread_t;

//------------------------------------------------------------------------------

static void connect()
{
    if (connected) return;

    {   // In case of a reconnect, previously enqueued messages are deleted
        ThreadLocker lock(bufferQueueMutex);

        while(!bufferQueue.empty())
        {
            delete[] bufferQueue.front();
            bufferQueue.pop();
        }
    }

    logInfo("TcpForwardPlugin: connecting to %s:%d", host.c_str(), port);

    lastConnectAttempt = time(0);

    socket_id = ::socket(AF_INET, SOCK_STREAM, 0);
    if (socket_id < 0)
    {
        logError("TcpForwardPlugin: failure to get a TCP socket");
        return;
    }

    hostent *server = gethostbyname(host.c_str());
    if (server == NULL)
    {
        logError("TcpForwardPlugin: failure to find server");
        throw runtime_error("TcpForwardPlugin: failure to find server");
    }

    sockaddr_in serv_addr;
    bzero((char *) &serv_addr, sizeof(serv_addr));
    serv_addr.sin_family = AF_INET;
    serv_addr.sin_port = htons(port);
    bcopy((char *)server->h_addr,
          (char *)&serv_addr.sin_addr.s_addr,
          server->h_length);


    int err = ::connect(socket_id, (sockaddr *) &serv_addr, sizeof(serv_addr));

    if (err)
    {
        logError("TcpForwardPlugin: failure to connect; errno = %d", errno);
        return;
    }

    logInfo("TcpForwardPlugin: connected to server");
    connected = true;
}

//------------------------------------------------------------------------------

static void reconnect()
{
    if (connected) return;

    if (difftime(time(0), lastConnectAttempt) < reconnectDelay) return;

    connect();
}

//------------------------------------------------------------------------------

static void *loop(void *arg)
{
    active = true;

    connect();

    while(active)
    {
        if (!connected) reconnect();

        if (!connected)
        {
            usleep(sleepDuration);
            continue;
        }

        unsigned char *buffer=0;
        {
            ThreadLocker lock(bufferQueueMutex);

            if (!bufferQueue.empty())
            {
                buffer = bufferQueue.front();
                bufferQueue.pop();
            }
        }

        if (buffer == 0)
        {
            usleep(sleepDuration);
            continue;
        }

        uint32_t const vmuSize = ntohl(*(uint32_t *)(buffer+8));
        uint32_t len = vmuSize+14; // 12 header bytes + 2 checksum bytes
        unsigned char *cursor = buffer;

        logInfo("TcpForwardPlugin: sending a %d-byte VMU packet", vmuSize);

        while (len>0)
        {
            int tx = ::send(socket_id, cursor, len, 0);

            if (tx == -1)
            {
                logError("TcpForwardPlugin: sending failure, errno=%d", errno);
                TcpForwardClient::disconnect();
                len = 0;
            } else {
                len -= tx;
                cursor += tx;
            }
        }

        delete[] buffer;
    }

    TcpForwardClient::disconnect();
    return 0;
}

//==============================================================================
// class TcpForwardClient
//==============================================================================

void TcpForwardClient::init(const string &hst, int prt, 
                            int reconDelay, unsigned int qSize)
{
    logInfo("TcpForwardPlugin: TCP/IP layer initialisation");

    host = hst;
    port = prt;
    reconnectDelay = reconDelay;
    queueSize = qSize;
}

//------------------------------------------------------------------------------

void TcpForwardClient::start()
{
    logInfo("TcpForwardPlugin: starting TCP layer thread");
    active = true;
    int err = pthread_create(sendThread, 0, &loop, 0);
    // Starts the instance thread.

    if (err)
    {
        logError("TcpForwardClient: TCP layer thread failed to start");
        logError("TcpForwardClient: pthread_create error code: %d", err);
        throw runtime_error("TcpForwardClient thread failed to start");
    }
}

//------------------------------------------------------------------------------

void TcpForwardClient::disconnect()
{
    if (!connected) return;

    connected = false;

    logInfo("TcpForwardPlugin: closing TCP connection");
    close(socket_id);
    socket_id = -1;
}

//------------------------------------------------------------------------------

void TcpForwardClient::shutdown()
{
    if (active)
    {
        logInfo("TcpForwardPlugin: shutting down TCP layer thread");
        active = false;
        pthread_join(*sendThread, 0);
    }
}

//------------------------------------------------------------------------------

void TcpForwardClient::send(unsigned char *buffer)
{
    if (active && connected)
    {
        ThreadLocker lock(bufferQueueMutex);
        if (queueSize != 0 && queueSize <= bufferQueue.size())
        {
            logInfo("TcpForwardPlugin: deleting oldest packet as queue is full");
            delete[] bufferQueue.front();
            bufferQueue.pop();
        }

        bufferQueue.push(buffer);
    }
    else
    {
        logInfo("TcpForwardPlugin: deleting packet as no available connection");
        delete[] buffer;
    }
}

//------------------------------------------------------------------------------

void TcpForwardClient::waitForConnection()
{

    if (! connected)
    {
        logInfo("TcpForwardPlugin: waiting for connection");

        while (! connected) usleep(sleepDuration);

        logInfo("TcpForwardPlugin: connected");
    }
}

//==============================================================================
