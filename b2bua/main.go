package main

import (
	"flag"
	"fmt"
	"go-sip-ua/b2bua/b2bua"
	"net/http"
	_ "net/http/pprof" // 导入 pprof 包，用于性能分析
	"os"
	"os/signal"
	"syscall"

	"github.com/c-bata/go-prompt"      // 导入 go-prompt 包，用于命令行交互
	"github.com/ghettovoice/gosip/log" // 导入 gosip 日志包
	"go-sip-ua/pkg/utils"              // 导入工具函数
)

// completer 提供命令行自动补全的建议
func completer(d prompt.Document) []prompt.Suggest {
	return prompt.FilterHasPrefix([]prompt.Suggest{
		{Text: "users", Description: "显示 SIP 账户"},
		{Text: "onlines", Description: "显示在线的 SIP 设备"},
		{Text: "calls", Description: "显示当前通话"},
		{Text: "set debug on", Description: "开启调试日志"},
		{Text: "set debug off", Description: "关闭调试日志"},
		{Text: "show loggers", Description: "打印日志记录器"},
		{Text: "exit", Description: "退出程序"},
	}, d.GetWordBeforeCursor(), true)
}

// usage 打印命令行使用说明
func usage() {
	fmt.Fprintf(os.Stderr, `go pbx 版本: go-pbx/1.10.0
用法: server [-nc]

选项:
`)
	flag.PrintDefaults()
}

// consoleLoop 运行命令行交互循环
func consoleLoop(b2bua *b2bua.B2BUA) {
	fmt.Println("请选择一个命令。")
	for {
		// 使用 go-prompt 实现命令行输入
		input := prompt.Input("CLI> ", completer,
			prompt.OptionTitle("GO B2BUA 1.0.0"),                        // 设置命令行标题
			prompt.OptionHistory([]string{"calls", "users", "onlines"}), // 设置历史命令
			prompt.OptionPrefixTextColor(prompt.Yellow),                 // 设置前缀文本颜色
			prompt.OptionPreviewSuggestionTextColor(prompt.Blue),        // 设置补全建议预览颜色
			prompt.OptionSelectedSuggestionBGColor(prompt.LightGray),    // 设置选中建议的背景颜色
			prompt.OptionSuggestionBGColor(prompt.DarkGray))             // 设置建议的背景颜色

		// 根据用户输入执行相应操作
		switch input {
		case "show loggers": // 显示日志记录器
			loggers := utils.GetLoggers() // 获取所有日志记录器
			for prefix, log := range loggers {
				fmt.Printf("%v => %v\n", prefix, log.Level()) // 打印日志记录器及其级别
			}
		case "set debug on": // 开启调试日志
			b2bua.SetLogLevel(log.DebugLevel) // 设置日志级别为 Debug
			fmt.Println("已设置日志级别为 debug")
		case "set debug off": // 关闭调试日志
			b2bua.SetLogLevel(log.WarnLevel) // 设置日志级别为 Info
			fmt.Println("已设置日志级别为 warn")
		case "users", "ul": // 显示 SIP 账户
			accounts := b2bua.GetAccounts() // 获取所有账户
			if len(accounts) > 0 {
				fmt.Println("用户:")
				fmt.Println("用户名 \t 密码")
				for user, pass := range accounts {
					fmt.Printf("%v \t\t %v\n", user, pass) // 打印用户名和密码
				}
			} else {
				fmt.Println("没有用户")
			}
		case "calls", "cl": // 显示当前通话
			calls := b2bua.Calls() // 获取所有通话
			if len(calls) > 0 {
				fmt.Println("通话:")
				for _, call := range calls {
					fmt.Printf("%v:\n", call.String()) // 打印通话信息
				}
			} else {
				fmt.Println("没有活跃的通话")
			}
		case "onlines", "rr": // 显示在线设备
			aors := b2bua.GetRegistry().GetAllContacts() // 获取所有注册记录
			if len(aors) > 0 {
				for aor, instances := range aors {
					fmt.Printf("AOR: %v:\n", aor) // 打印 AOR（Address of Record）
					for _, instance := range instances {
						fmt.Printf("\t%v, 过期时间: %d, 来源: %v, 传输协议: %v\n",
							(*instance).UserAgent, (*instance).RegExpires, (*instance).Source, (*instance).Transport)
					}
				}
			} else {
				fmt.Println("没有在线的设备")
			}
		case "exit": // 退出程序
			fmt.Println("正在退出...")
			b2bua.Shutdown() // 关闭 B2BUA
			return
		}
	}
}

func main() {
	var (
		noconsole   bool // 是否禁用命令行交互模式
		disableAuth bool // 是否禁用认证
		enableTLS   bool // 是否启用 TLS
		h           bool // 是否显示帮助信息
	)
	flag.BoolVar(&h, "h", false, "显示帮助信息")
	flag.BoolVar(&noconsole, "nc", false, "禁用命令行交互模式")
	flag.BoolVar(&disableAuth, "da", false, "禁用认证")
	flag.BoolVar(&enableTLS, "tls", false, "启用 TLS")
	flag.Usage = usage // 设置帮助信息函数
	flag.Parse()       // 解析命令行参数

	if h { // 如果用户请求帮助信息
		flag.Usage() // 显示帮助信息
		return
	}

	stop := make(chan os.Signal, 1)                      // 创建一个信号通道
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT) // 监听 SIGTERM 和 SIGINT 信号

	go func() {
		fmt.Println("正在启动 pprof，端口 :6658")
		http.ListenAndServe(":6658", nil) // 启动 HTTP 服务器，用于性能分析
	}()

	b2bua := b2bua.NewB2BUA(disableAuth, enableTLS) // 创建 B2BUA 实例

	// 添加示例账户
	b2bua.AddAccount("100", "100")
	b2bua.AddAccount("200", "200")

	if !noconsole { // 如果未禁用命令行交互模式
		consoleLoop(b2bua) // 进入命令行交互循环
		return
	}

	<-stop           // 等待信号
	b2bua.Shutdown() // 关闭 B2BUA
}
