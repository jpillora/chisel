package chshare

//this file exists to maintain backwards compatibility

import (
	"github.com/jpillora/chisel/share/ccrypto"
	"github.com/jpillora/chisel/share/cio"
	"github.com/jpillora/chisel/share/cnet"
	"github.com/jpillora/chisel/share/config"
	"github.com/jpillora/chisel/share/cos"
)

const (
	DetermRandIter = ccrypto.DetermRandIter
)

type (
	Config     = config.Config
	Remote     = config.Remote
	Remotes    = config.Remotes
	User       = config.User
	Users      = config.Users
	UserIndex  = config.UserIndex
	HTTPServer = cnet.HTTPServer
	ConnStats  = cnet.ConnStats
	Logger     = cio.Logger
)

var (
	NewDetermRand    = ccrypto.NewDetermRand
	GenerateKey      = ccrypto.GenerateKey
	FingerprintKey   = ccrypto.FingerprintKey
	Pipe             = cio.Pipe
	NewLoggerFlag    = cio.NewLoggerFlag
	NewLogger        = cio.NewLogger
	Stdio            = cio.Stdio
	EncodeConfig     = config.EncodeConfig
	DecodeConfig     = config.DecodeConfig
	DecodeRemote     = config.DecodeRemote
	NewUsers         = config.NewUsers
	NewUserIndex     = config.NewUserIndex
	UserAllowAll     = config.UserAllowAll
	ParseAuth        = config.ParseAuth
	NewRWCConn       = cnet.NewRWCConn
	NewWebSocketConn = cnet.NewWebSocketConn
	NewHTTPServer    = cnet.NewHTTPServer
	GoStats          = cos.GoStats
	SleepSignal      = cos.SleepSignal
)
