[![Go Report Card](https://goreportcard.com/badge/github.com/busoc/hadock)](https://goreportcard.com/report/github.com/busoc/hadock)

the high rate data brocker AKA hadock has been developped to support high rate data
activities in the frame of FSL operations at B.USOC.

main features of hadock are:

* processing of HRDL packets
  - storage in local archive
  - detection of invalid/corrupted packets
  - creation of processed parameters
  - cascading
* HTTP interface for clients to get raw or processed data archived by hadock
* monitoring of HRDL stream and hadock activities
* manual replay of RT files from HRDP archive.

Each of these features have a dedicated command built-in into hadock each
summarized below .

the ``listen`` command implements the core of hadock: processing and storing
incoming HRDL packets. this command expect that each HRDL packets are embedded
in the hadock own protocol.

For each packets received, hadock will:

* identifiy the instance, mode, type of a packet,
* store the raw data in a dedicated directory with a configurable directory tree structure according to the values found in the headers of the HRDL packet,
* store metadata (XML format) next to the raw data when a packet is an image packet,
* summarize the status of the HRDL stream and generate processed parameters at
regular interval for monitoring tools.

hadock can support multiple type of storage:

* file: data from HRDL packets are stored in individual file
* tar: data from HRDL packets are stored in tar archive that will be automatically
rotated under certain conditions
* hrdp: each HRDL packets are stored in files similar to the RT files found in
the HRDP archive.

the ``replay`` command has been initially written to develop and test the protocol
used by  hadock ``listen`` in order to process incoming HRDL packets.

Today, its usage has been extended to:

* validation of new feature of hadock,
* consolidation of hadock archives.

the ``distrib`` command runs a embedded HTTP server (that should be run behind a
nginx or apache httpd server) to deliver data to clients (external tools or users).

This command used the raw archive of hadock as its source to deliver the data.
For every request received, it will read the raw data and try to convert it into
a readable format (eg: csv for sciences data, png|jpg for image data).

additional tools have been developped in the meantime and are available in their
own dedicated repositories. These tools can be used for different purposes such as:

* scanning the hadock archive ([upifinder](https://github.com/busoc/upifinder))
* scanning the HRDP archive for VMU packets ([meex](https://github.com/busoc/meex))
* feeds hadock with HRDL packets from a stream of VCDU ([erdle](https://github.com/busoc/erdle))
* process MVIS data ([mvis2list](https://github.com/busoc/mvis2list))
