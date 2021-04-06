ifndef STANDALONE

# The environment variable DASSROOT must be set before running make.
include $(DASSROOT)/cpp/make/dass_client_api.inc
INCLUDES+=-I$(DASSROOT)/cpp/apis/dass/hrdl
INCLUDES+=-I$(DASSROOT)/cpp/apis/dass
INCLUDES+=-I$(DASSROOT)/cpp/Util

all: TcpForwardPlugin.so

else

CPPFLAGS+=-DSTANDALONE_BUILD
all: TcpForwardPlugin.so TestDriver

endif

# Automatic dependency generation.
# See http://make.mad-scientist.net/papers/advanced-auto-dependency-generation/
DEPDIR := .dependencies
$(shell mkdir -p $(DEPDIR) >/dev/null)
DEPFLAGS = -MT $@ -MMD -MP -MF $(DEPDIR)/$*.Td
POSTCOMPILE = @mv -f $(DEPDIR)/$*.Td $(DEPDIR)/$*.d && touch $@

VERSION=1.0.0

ALL_SRCS=$(wildcard *.cpp)
PLUGIN_SRCS=$(wildcard TcpForward*.cpp)
PLUGIN_OBJS=$(foreach src,$(PLUGIN_SRCS),$(src:.cpp=.o))

INCLUDES+=-ISE
CPPFLAGS+= -fPIC -g -Wall ${DEPFLAGS}


%.o : %.cpp
%.o : %.cpp $(DEPDIR)/%.d
	g++ $(CPPFLAGS) $(INCLUDES) -c $< -o $*.o
	$(POSTCOMPILE)


$(DEPDIR)/%.d: ;
.PRECIOUS: $(DEPDIR)/%.d

TcpForwardPlugin.so: $(PLUGIN_OBJS)
	g++ -shared -Wl,-soname,TcpForwardPlugin-$(VERSION).so -o TcpForwardPlugin-$(VERSION).so $^ -lc
	ln -sf TcpForwardPlugin-$(VERSION).so TcpForwardPlugin.so

TestDriver: TestDriver.o
	g++ TcpForwardPlugin.so -lpthread $^ -o $@


clean:
	rm -rf *.o *.so $(DEPDIR) TestDriver

include $(wildcard $(patsubst %,$(DEPDIR)/%.d,$(basename $(ALL_SRCS))))
