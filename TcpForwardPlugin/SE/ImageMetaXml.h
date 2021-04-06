#ifndef IMAGEMETAXML_H_
#define IMAGEMETAXML_H_

#include <string>
#include "ArgumentHandler.h"
#include "CCSDSUTime.h"

using std::string;

class ImageMetaXml
{
private:
	struct Arguments const * const lArgs;
    static long EPOCH_DIFF_S;
    
	inline string getItemKeyXml(char const * key, string& value) { return getItemKeyXml(key, value.c_str()); }
	string getItemKeyXml(char const * key, char const * value);
public:
	ImageMetaXml(struct Arguments const * const ah);
	virtual ~ImageMetaXml();	
	
	string createMetaXml(
		string filename,
		string metaFilename,
		char * const time,
		string s_HCI, 
		string s_SID, 
		string s_SEQ, 
		string s_TIM, 
		string s_SEQviph, 
		string s_TIMviph, 
		string s_POR, 
		string s_VID, 
		string s_TYP, 
		string s_FRT, 
		string s_PIXpx, 
		string s_PIXpy, 
		string s_ROIxof, 
		string s_ROIxsz, 
		string s_ROIyof, 
		string s_ROIysz, 
		string s_DRPldrp, 
		string s_DRPfdrp, 
		string s_UPI);

	string createMetaXmlForScienceDataFile(
		string filename,
        string onboardFilename,
		string metaFilename,
		char * const time,
		string s_HCI, 
		string s_SID, 
		string s_SEQ, 
		string s_TIM, 
		string s_SEQsdph);
	
	static string getMissionModeText(int const mm);
	static long getGpsToUnixEpoch();
};

#endif /*IMAGEMETAXML_H_*/
