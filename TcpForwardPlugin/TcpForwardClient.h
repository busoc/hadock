//==============================================================================
// TcpForwardClient.h
//==============================================================================
//
// Author: Didier Pieroux (BIRA-IASB)
//
// Implementation of the TCP client used by the TcpForwardPlugin instances.
//
// See TcpForwardPlugin.txt for an overview of the implementation.
//==============================================================================

#ifndef TCP_FORWARD_CLIENT_H
#define TCP_FORWARD_CLIENT_H

#include <string>
#include <vector>

struct Arguments;


//==============================================================================
// class TcpForwardClient
//
// Specific TCP client class to be used by the plug-in.
//==============================================================================

class TcpForwardClient
{
public:
    static void init(const string &host, int port, 
                     int reconnectDelay, unsigned int qSize);
    // Initialise the TCP layer.

    static void start();
    // Start the internal thread.
    //
    // Throws runtime_error is the thread doesn't start.

    static void shutdown();
    // Stop the internal thread.

    static void send(unsigned char *buf);
    // Enqueues a buffer for sending.


    static void waitForConnection();
    // Wait till the TCP connection is established. Note: this function is only
    // for testing purpose. Do not use it the plugin-in production code as
    // it is a blocking call.

    static void disconnect();
    // Close the TCP connection. Note: this function is made public for testing
    // purpose only.

private:
    TcpForwardClient();
};

//==============================================================================

#endif // TCP_FORWARD_CLIENT_H
