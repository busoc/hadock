#! /bin/bash

# retrieve hadock version using git tag
VERSION=$(git tag | tail  -n 1)

#get go compiler version 
COMPILER=$(go version | cut -f 3 -d ' ')

# get fully qualified hostname
HOST=$(hostname -f)

# get current date (utc)
DATE=$(date -u '+%Y-%m-%d %H:%M:%S')

FILE=bin/hadock
if [[ -n $1 ]]; then
	FILE=$1
fi

VERSION=${VERSION#v}
if [[ -z $VERSION ]]; then
	VERSION="devel"
fi


#FLAGS="-extldflags '-static' -X 'github.com/midbel/cli.Version=${VERSION}' -X 'github.com/midbel/cli.BuildTime=${DATE}' -X 'github.com/midbel/cli.CompileWith=${COMPILER}'"
FLAGS="-X 'github.com/midbel/cli.Version=${VERSION}' -X 'github.com/midbel/cli.BuildTime=${DATE}' -X 'github.com/midbel/cli.CompileWith=${COMPILER}' -X 'github.com/midbel/cli.CompileHost=${HOST}'"

rm -rf $FILE
go build -ldflags "${FLAGS}" -o $FILE cmd/hadock/*go
