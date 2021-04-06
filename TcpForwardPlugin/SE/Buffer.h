#ifndef BUFFER_H_
#define BUFFER_H_

class Buffer
{
private:
	unsigned char * const lBuffer;
	unsigned int lLength;
	
public:
	Buffer(unsigned char * buf, unsigned int length);
	Buffer(Buffer const & buf);
	virtual ~Buffer();
	
	unsigned char * getBuffer();
	unsigned int getLength();
};

#endif /*BUFFER_H_*/
