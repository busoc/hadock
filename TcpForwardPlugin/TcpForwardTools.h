//==============================================================================
// TcpForwardTools.h
//==============================================================================
//
// Author: Didier Pieroux (BIRA-IASB)
//
// Tools used by the TcpForwardPlugin implementation.
//
//==============================================================================

#ifndef TCP_FORWARD_TOOLS_H
#define TCP_FORWARD_TOOLS_H

#include <pthread.h>

//==============================================================================
// class ThreadLocker
//
// RAII implementation for mutexes.
//==============================================================================

class ThreadLocker
{
public:
    ThreadLocker(pthread_mutex_t *mutex);
    // Locks the mutex.

    ~ThreadLocker();
    // Unlocks the mutex.

private:
    pthread_mutex_t *mutex;
};


//==============================================================================
// Tools
//==============================================================================

extern pthread_mutex_t *new_pthread_mutex_t();
// Returns an initialised mutex.

//------------------------------------------------------------------------------

extern uint16_t ip_checksum(const unsigned char *data, size_t length);
// Compute the checksum of a buffer.

//==============================================================================

#endif // TCP_FORWARD_TOOLS_H
