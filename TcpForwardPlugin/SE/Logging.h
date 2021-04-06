#ifndef LOGGING_H_
#define LOGGING_H_
void
startLogging();

void
stopLogging();

//inline
extern
void
logInfo(const char * format, ...);

//inline
extern
void
logError(const char * format, ...);

//Not to be used by plugins
//inline
extern
void
logInfoSE(char * format, ...);

//Not to be used by plugins
//inline
extern
void
logErrorSE(char * format, ...);

extern
void
setLoggingServiceMode(int serviceMode);

#endif /*LOGGING_H_*/
