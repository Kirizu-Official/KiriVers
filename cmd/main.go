package cmd

import (
	"errors"
	"fmt"
	"os"
)

func Run() {
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		fatalUsage(err)
	}

	switch commandOf(args.positional) {
	case "server":
		// -password 只属于 admin 子命令，传给 server 视为拼写错误。
		if args.password != "" {
			fatalUsage(errors.New("-password is only valid with the admin command"))
		}
		runServer(resolveConfigPath(args.configPath))
	case "admin":
		runAdmin(args)
	case "help":
		usage()
	default:
		fatalUsage(fmt.Errorf("unknown command %q", args.positional[0]))
	}
}

// commandOf 决定运行模式：无参数或首参数为 server 时默认启动 HTTP 服务端。
func commandOf(positional []string) string {
	if len(positional) == 0 || positional[0] == "server" {
		return "server"
	}
	return positional[0]
}

// resolveConfigPath 应用配置来源优先级：-config 标志优先于 KIRIVERS_CONFIG 环境变量。
func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("KIRIVERS_CONFIG")
}

func usage() {
	fmt.Fprintf(os.Stderr, `KiriVers — unified startup entry

Usage:
  kirivers [server] [-config path]                              start the HTTP servers (default)
                                                                client plane (update/store/telemetry) on client.yaml addr (:8080)
                                                                admin plane (admin console + CI agent) on admin.yaml addr (:8081)
  kirivers admin add <username> [-password secret]              create an instance admin
  kirivers admin delete <username>                              delete an instance admin
  kirivers admin list                                           list instance admins
  kirivers admin reset-password <username> [-password secret]   reset an admin password
  kirivers admin clear-2fa <username>                           clear TOTP, passkeys and recovery codes (does not revoke sessions)

Optional: -config path-or-directory
          directory: config.yaml + admin.yaml + client.yaml in that folder
          file: that system YAML plus sibling admin.yaml and client.yaml
          default: configs/ then current dir; missing files fall back to *-example.yaml
          KIRIVERS_CONFIG is honored when -config is absent
          kirivers admin loads only the system file
`)
}

// fatalUsage 打印错误与用法后以 2 退出（参数/用法错误）。
func fatalUsage(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	usage()
	os.Exit(2)
}
