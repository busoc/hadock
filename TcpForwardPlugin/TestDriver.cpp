#include "ArgumentHandler.h"
#include "TcpForwardPlugin.h"
#include "TcpForwardClient.h"

#include <stdint.h>
#include <cstdarg>
#include <fstream>
#include <string>
#include <unistd.h>

using namespace std;

void logInfo(const char * format, ...)
{
    va_list arguments;
    va_start(arguments, format);
    fprintf(stderr, "INFO:  ");
    vfprintf(stderr, format, arguments);
    fprintf(stderr, "\n");
    va_end(arguments);
}

void logError(const char * format, ...)
{
    va_list arguments;
    va_start(arguments, format);
    fprintf(stderr, "ERROR: ");
    vfprintf(stderr, format, arguments);
    fprintf(stderr, "\n");
    va_end(arguments);
}

void display_error_msg()
{
    cout <<
    "Standalone test driver of the TcpForwardPlugin.\n"
    "\n"
    "Usage: \n"
    "    TestDriver n \"host:port, reconTime, queueSize\" [inFile] \n"
    "\n"
    "Arguments:\n"
    "    n:         reference of the test; n=1, 2 or 3\n"
    "    host:      address of the TCP server to connect to\n"
    "    port:      port of the TCP server to connect to\n"
    "    reconTime: delay in seconds before trying to reconnect\n"
    "    queueSize: size of the message queue; 0 means unlimited\n"
    "    infile:    file containing a sequence of VMU records; only for n=3\n";
}

//------------------------------------------------------------------------------

int test1(int argc, char* argv[])
{
    Arguments args;
    args.MissionMode = 1;

    if (argc != 3)
    {
        display_error_msg();
        return -1;
    }

    args.pluginParams = argv[2];

    VMUDPPlugin *plugin = (VMUDPPlugin *) Init(&args);

    TcpForwardClient::waitForConnection();

    TcpForwardClient::disconnect();

    TcpForwardClient::waitForConnection();

    Reset(plugin);

    return 0;
}

//------------------------------------------------------------------------------

int test2(int argc, char* argv[])
{
    Arguments args;
    args.MissionMode = 1;

    if (argc != 3)
    {
        display_error_msg();
        return -1;
    }

    args.pluginParams = argv[2];

    VMUDPPlugin *plugin = (VMUDPPlugin *) Init(&args);

    TcpForwardClient::waitForConnection();

    for(int i=0; i<10; ++i)
        TcpForwardPlugin::handlePacket((const unsigned char*)"Hello World!\n", 13);

    usleep(500000); // wait half a second
    Reset(plugin);

    return 0;
}

//------------------------------------------------------------------------------

int test3(int argc, char* argv[])
{
    Arguments args;
    args.MissionMode = 1;

    if (argc != 4)
    {
        display_error_msg();
        return -1;
    }

    args.pluginParams = argv[2];

    VMUDPPlugin *plugin = (VMUDPPlugin *) Init(&args);

    TcpForwardClient::waitForConnection();

    ifstream infile(argv[3], ios::binary);

    char header[26];

    while (infile)
    {

        infile.read(header, 26);
        uint32_t syncWord = *(uint32_t *)(header+18);
        uint32_t len = *(uint32_t *)(header+22);

        if (syncWord != 0x53352ef8)
        {
            logError("Bad input file; sync word not found.");
            break;
        }

        char *buffer = new char[len];

        infile.read(buffer, len);

        TcpForwardPlugin::handlePacket((unsigned char *)buffer, len);
        delete[] buffer;

        infile.read(header, 4);

    }

    Reset(plugin);

    return 0;
}

//------------------------------------------------------------------------------

int main(int argc, char* argv[])
{
    if (argc < 2)
    {
        display_error_msg();
        return -1;
    }

    istringstream argv1(argv[1]);
    int testNumber;

    argv1 >> testNumber;

    switch(testNumber)
    {
        case 1:
            return test1(argc, argv);
            break;

        case 2:
            return test2(argc, argv);
            break;

        case 3:
            return test3(argc, argv);
            break;

        default:
            display_error_msg();
            return -1;
    }

    return -1;
}
