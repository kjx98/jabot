#
#	Makefile for hookAPI
#
# switches:
#	define the ones you want in the CFLAGS definition...
#
#	TRACE		- turn on tracing/debugging code
#
#
#
#

# Version for distribution
VER=1_0r1

MAKEFILE=GNUmakefile

# We Use Compact Memory Model

all: bin/rebot bin/example
	@[ -d bin ] || exit

example: bin/example

bin/rebot: rebot/main.go config.go jabot.go
	@[ -d bin ] || mkdir bin
	go build -o bin/rebot rebot/main.go
	@strip $@ || echo "rebot OK"

bin/example: example/example.go config.go jabot.go
	@[ -d bin ] || mkdir bin
	go build -o bin/example example/example.go

win64: bin/rebot.exe

bin/rebot.exe: rebot/main.go config.go jabot.go
	(GOOS=windows GOARCH=amd64 go build -o $@ rebot/main.go)
	@strip $@ || echo "rebot.exe win64 OK"

clean:

distclean: clean
	@rm -rf bin
