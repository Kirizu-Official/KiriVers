// admin 子命令：本机管理员账号的添加、删除、列表与重置密码。直接写数据库，不走 HTTP。
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rs/zerolog"
	"golang.org/x/term"

	"github.com/Kirizu-Official/KiriVers/internal/config"
	"github.com/Kirizu-Official/KiriVers/internal/database"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

// runAdmin 执行 admin 子命令（args.positional[0] == "admin"）。
func runAdmin(args *cliArgs) {
	positional := args.positional
	if len(positional) < 2 {
		fatalUsage(errors.New("admin requires an action"))
	}

	cfg, err := config.LoadSystem(resolveConfigPath(args.configPath))
	if err != nil {
		fatal(err)
	}
	db, err := database.Open(cfg.Postgres.DSN, zerolog.Nop())
	if err != nil {
		fatal(err)
	}
	defer database.Close(db)
	if err := database.AutoMigrate(db); err != nil {
		fatal(err)
	}

	admins := service.NewAdminService(service.AdminServiceOptions{
		Store: repository.NewAdminRepo(db),
		TwoFA: repository.NewAdmin2FARepo(db),
	})
	ctx := context.Background()
	action := positional[1]
	rest := positional[2:]

	switch action {
	case "add":
		username := requireName(rest, "kirivers admin add <username>")
		pass, err := resolvePassword(args.password, true)
		if err != nil {
			fatal(err)
		}
		admin, err := admins.Create(ctx, username, pass)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("created %s (%s)\n", admin.Username, admin.ID)
	case "delete":
		username := requireName(rest, "kirivers admin delete <username>")
		if err := admins.DeleteByUsername(ctx, username); err != nil {
			fatal(err)
		}
		fmt.Printf("deleted %s\n", username)
	case "list":
		list, err := admins.List(ctx)
		if err != nil {
			fatal(err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "USERNAME\tID\tCREATED_AT")
		for _, a := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\n", a.Username, a.ID, a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
		}
		_ = w.Flush()
	case "reset-password":
		username := requireName(rest, "kirivers admin reset-password <username>")
		pass, err := resolvePassword(args.password, true)
		if err != nil {
			fatal(err)
		}
		if err := admins.ResetPassword(ctx, username, pass); err != nil {
			fatal(err)
		}
		fmt.Printf("password reset for %s\n", username)
	case "clear-2fa":
		username := requireName(rest, "kirivers admin clear-2fa <username>")
		if err := admins.Clear2FA(ctx, username); err != nil {
			fatal(err)
		}
		fmt.Printf("cleared 2FA for %s\n", username)
	default:
		fatalUsage(fmt.Errorf("unknown admin action %q", action))
	}
}

// cliArgs 是统一解析后的命令行输入：全局标志加位置参数。
type cliArgs struct {
	configPath string
	password   string
	positional []string
}

// parseArgs 解析全局标志（-config / -password）与位置参数，标志可出现在任意位置。
func parseArgs(argv []string) (*cliArgs, error) {
	args := &cliArgs{}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "-h" || a == "--help":
			args.positional = []string{"help"}
			return args, nil
		case a == "-config" || a == "--config":
			if i+1 >= len(argv) {
				return nil, errors.New("-config requires a path")
			}
			i++
			args.configPath = argv[i]
		case strings.HasPrefix(a, "-config="):
			args.configPath = strings.TrimPrefix(a, "-config=")
		case a == "-password" || a == "--password":
			if i+1 >= len(argv) {
				return nil, errors.New("-password requires a value")
			}
			i++
			args.password = argv[i]
		case strings.HasPrefix(a, "-password="):
			args.password = strings.TrimPrefix(a, "-password=")
		case strings.HasPrefix(a, "-"):
			return nil, fmt.Errorf("unknown flag %s", a)
		default:
			args.positional = append(args.positional, a)
		}
	}
	return args, nil
}

func requireName(rest []string, usageLine string) string {
	if len(rest) < 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Fprintf(os.Stderr, "usage: %s\n", usageLine)
		os.Exit(2)
	}
	return strings.TrimSpace(rest[0])
}

func resolvePassword(flagValue string, confirm bool) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("no TTY: pass -password")
	}
	fmt.Fprint(os.Stderr, "Password: ")
	b1, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if confirm {
		fmt.Fprint(os.Stderr, "Confirm: ")
		b2, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(b1) != string(b2) {
			return "", errors.New("passwords do not match")
		}
	}
	return string(b1), nil
}

// fatal 打印错误后以 1 退出（运行期不可恢复错误）。
func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
