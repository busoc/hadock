#ifndef ARGUMENTHANDLER_H_
#define ARGUMENTHANDLER_H_

#include <iostream>
#include <sstream>

using std::string;
using std::ostringstream;

//Struct for parsing command line opts.
struct Arguments {
	ostringstream Hostname;
	int ClientPort;
	
	ostringstream ClientName;
	ostringstream ClientPassword;
	int RequestedDataType;
	
	int ServiceMode;
	string PlaybackStart;
	string PlaybackEnd;
	
	int MissionMode;
	string MissionModeStr;
	int ExtractMode;
	string MetadataDescription;

	ostringstream MulticastAddress;
	int MulticastPort;
	ostringstream OutputDirectory;
	
	int SessionRetrievalTime;
	int ConnectTimeOut;
	int ServiceRetryTime;
	int StreamServiceRestartTime;
	
	int InactivityConnectionRestartTime;
	/** Multicaster settings */
	bool EnableMulticast;
	bool EnableMulticastSmoothAlgorithm;
	int MulticastUdpMaxSize;
	string provider;
	string retrievalRole;
	string modifyRole;
	
	bool UseHrdlTime;
	bool ExtraDebugInfo;
	/* 
	* Added for Plugin.
	*/
	std::string pluginFile;
	std::string pluginParams;
};

class ArgumentHandler
{
private:
	struct Arguments args;
	
	//private methods
	void initArguments(struct Arguments & args);
   	int parseOneArg(int const argc, char * argv[], int index);
   		
public:
	ArgumentHandler();
	virtual ~ArgumentHandler();
	
	bool parseArguments(int const , char * argv[] );
	
	struct Arguments* getArguments();
};

#endif /*ARGUMENTHANDLER_H_*/
