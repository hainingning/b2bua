package b2bua

import (
	"fmt"
	registry2 "go-sip-ua/b2bua/registry"

	"github.com/ghettovoice/gosip/log"        // 导入日志模块
	"github.com/ghettovoice/gosip/sip"        // 导入 SIP 协议模块
	"github.com/ghettovoice/gosip/sip/parser" // 导入 SIP 解析模块
	"github.com/ghettovoice/gosip/transport"  // 导入传输模块
	"go-sip-ua/pkg/account"                   // 导入账户管理模块
	"go-sip-ua/pkg/auth"                      // 导入认证模块
	"go-sip-ua/pkg/session"                   // 导入会话管理模块
	"go-sip-ua/pkg/stack"                     // 导入 SIP 协议栈模块
	"go-sip-ua/pkg/ua"                        // 导入用户代理模块
	"go-sip-ua/pkg/utils"                     // 导入工具模块
)

// B2BCall 表示一个 B2BUA 呼叫，包含源会话和目标会话
type B2BCall struct {
	src  *session.Session // 源会话
	dest *session.Session // 目标会话
}

// String 返回 B2BCall 的字符串表示
func (b *B2BCall) String() string {
	return fmt.Sprintf("%s => %s", b.src.Contact(), b.dest.Contact())
}

// B2BUA 表示 B2BUA 的核心逻辑
type B2BUA struct {
	stack    *stack.SipStack    // SIP 协议栈
	ua       *ua.UserAgent      // 用户代理
	accounts map[string]string  // 账户信息（用户名 -> 密码）
	registry registry2.Registry // 注册管理
	domains  []string           // 域名列表
	calls    []*B2BCall         // 当前通话列表
}

var (
	logger log.Logger // 日志记录器
)

func init() {
	logger = utils.NewLogrusLogger(log.InfoLevel, "B2BUA", nil) // 初始化日志记录器
}

// NewB2BUA 创建一个新的 B2BUA 实例
func NewB2BUA(disableAuth, enableTLS bool) *B2BUA {
	b := &B2BUA{
		registry: registry2.NewMemoryRegistry(), // 初始化内存注册表
		accounts: make(map[string]string),       // 初始化账户信息
	}

	var authenticator *auth.ServerAuthorizer
	if !disableAuth { // 如果未禁用认证
		authenticator = auth.NewServerAuthorizer(b.requestCredential, "b2bua", false) // 创建认证器
	}

	// 初始化 SIP 协议栈
	stack := stack.NewSipStack(&stack.SipStackConfig{
		UserAgent:  "Go B2BUA/1.0.0",                 // 用户代理标识
		Extensions: []string{"replaces", "outbound"}, // 支持的扩展
		Dns:        "8.8.8.8",                        // DNS 服务器
		ServerAuthManager: stack.ServerAuthManager{
			Authenticator:     authenticator,       // 认证器
			RequiresChallenge: b.requiresChallenge, // 是否需要挑战
		},
	})

	stack.OnConnectionError(b.handleConnectionError) // 设置连接错误处理函数

	// 监听 UDP 端口
	if err := stack.Listen("udp", "0.0.0.0:5060"); err != nil {
		logger.Panic(err)
	}

	// 监听 TCP 端口
	if err := stack.Listen("tcp", "0.0.0.0:5060"); err != nil {
		logger.Panic(err)
	}

	if enableTLS { // 如果启用 TLS
		tlsOptions := &transport.TLSConfig{Cert: "certs/cert.pem", Key: "certs/key.pem"} // TLS 配置
		if err := stack.ListenTLS("tls", "0.0.0.0:5061", tlsOptions); err != nil {       // 监听 TLS 端口
			logger.Panic(err)
		}
		if err := stack.ListenTLS("wss", "0.0.0.0:5081", tlsOptions); err != nil { // 监听 WSS 端口
			logger.Panic(err)
		}
	}

	// 初始化用户代理
	ua := ua.NewUserAgent(&ua.UserAgentConfig{
		SipStack: stack, // 绑定 SIP 协议栈
	})

	// 设置 INVITE 状态处理函数
	ua.InviteStateHandler = func(sess *session.Session, req *sip.Request, resp *sip.Response, state session.Status) {
		logger.Infof("InviteStateHandler: state => %v, type => %s", state, sess.Direction())

		switch state {
		case session.InviteReceived: // 收到 INVITE 请求
			to, _ := (*req).To()
			from, _ := (*req).From()
			caller := from.Address
			called := to.Address

			doInvite := func(instance *registry2.ContactInstance) {
				displayName := ""
				if from.DisplayName != nil {
					displayName = from.DisplayName.String()
				}

				profile := account.NewProfile(caller, displayName, nil, 0, stack)

				recipient, err := parser.ParseSipUri("sip:" + called.User().String() + "@" + instance.Source + ";transport=" + instance.Transport)
				if err != nil {
					logger.Error(err)
				}

				offer := sess.RemoteSdp()
				dest, err := ua.Invite(profile, called, recipient, &offer)
				if err != nil {
					logger.Errorf("B-Leg session error: %v", err)
					return
				}
				b.calls = append(b.calls, &B2BCall{src: sess, dest: dest})
			}

			if contacts, found := b.registry.GetContacts(called); found { // 查找被叫方的注册信息
				sess.Provisional(100, "Trying")
				for _, instance := range *contacts {
					doInvite(instance)
				}
				return
			}

			sess.Reject(404, fmt.Sprintf("%v Not found", called)) // 如果未找到被叫方，返回 404

		case session.ReInviteReceived: // 收到 re-INVITE 请求
			logger.Infof("re-INVITE")
			switch sess.Direction() {
			case session.Incoming:
				sess.Accept(200)
			case session.Outgoing:
				// TODO: 提供适当的响应
			}

		case session.EarlyMedia, session.Provisional: // 早期媒体或临时响应
			call := b.findCall(sess)
			if call != nil && call.dest == sess {
				answer := call.dest.RemoteSdp()
				call.src.ProvideAnswer(answer)
				call.src.Provisional((*resp).StatusCode(), (*resp).Reason())
			}

		case session.Confirmed: // 会话确认
			call := b.findCall(sess)
			if call != nil && call.dest == sess {
				answer := call.dest.RemoteSdp()
				call.src.ProvideAnswer(answer)
				call.src.Accept(200)
			}

		case session.Failure, session.Canceled, session.Terminated: // 会话失败、取消或终止
			call := b.findCall(sess)
			if call != nil {
				if call.src == sess {
					call.dest.End()
				} else if call.dest == sess {
					call.src.End()
				}
			}
			b.removeCall(sess)
		}
	}

	// 设置注册状态处理函数
	ua.RegisterStateHandler = func(state account.RegisterState) {
		logger.Infof("RegisterStateHandler: state => %v", state)
	}

	stack.OnRequest(sip.REGISTER, b.handleRegister) // 设置 REGISTER 请求处理函数
	b.stack = stack
	b.ua = ua
	return b
}

// Calls 返回当前的通话列表
func (b *B2BUA) Calls() []*B2BCall {
	return b.calls
}

// findCall 根据会话查找通话
func (b *B2BUA) findCall(sess *session.Session) *B2BCall {
	for _, call := range b.calls {
		if call.src == sess || call.dest == sess {
			return call
		}
	}
	return nil
}

// removeCall 根据会话移除通话
func (b *B2BUA) removeCall(sess *session.Session) {
	for idx, call := range b.calls {
		if call.src == sess || call.dest == sess {
			b.calls = append(b.calls[:idx], b.calls[idx+1:]...)
			return
		}
	}
}

// Shutdown 关闭 B2BUA
func (b *B2BUA) Shutdown() {
	b.ua.Shutdown()
}

// requiresChallenge 检查请求是否需要挑战
func (b *B2BUA) requiresChallenge(req sip.Request) bool {
	switch req.Method() {
	case sip.REGISTER, sip.INVITE: // REGISTER 和 INVITE 请求需要挑战
		return true
	case sip.CANCEL, sip.OPTIONS, sip.INFO, sip.BYE: // 其他请求不需要挑战
		return false
	}
	return false
}

// AddAccount 添加一个 SIP 账户
func (b *B2BUA) AddAccount(username, password string) {
	b.accounts[username] = password
}

// GetAccounts 返回所有 SIP 账户
func (b *B2BUA) GetAccounts() map[string]string {
	return b.accounts
}

// GetRegistry 返回注册管理
func (b *B2BUA) GetRegistry() registry2.Registry {
	return b.registry
}

// requestCredential 根据用户名获取凭证
func (b *B2BUA) requestCredential(username string) (string, string, error) {
	if password, found := b.accounts[username]; found {
		logger.Infof("Found user %s", username)
		return password, "", nil
	}
	return "", "", fmt.Errorf("username [%s] not found", username)
}

// handleRegister 处理 REGISTER 请求
func (b *B2BUA) handleRegister(request sip.Request, tx sip.ServerTransaction) {
	headers := request.GetHeaders("Expires")
	to, _ := request.To()
	aor := to.Address.Clone()
	var expires sip.Expires = 0
	if len(headers) > 0 {
		expires = *headers[0].(*sip.Expires)
	}

	reason := ""
	if len(headers) > 0 && expires != sip.Expires(0) {
		instance := registry2.NewContactInstanceForRequest(request)
		logger.Infof("Registered [%v] expires [%d] source %s", to, expires, request.Source())
		reason = "Registered"
		b.registry.AddAor(aor, instance)
	} else {
		logger.Infof("Logged out [%v] expires [%d] ", to, expires)
		reason = "UnRegistered"
		instance := registry2.NewContactInstanceForRequest(request)
		b.registry.RemoveContact(aor, instance)
	}

	resp := sip.NewResponseFromRequest(request.MessageID(), request, 200, reason, "")
	sip.CopyHeaders("Expires", request, resp)
	utils.BuildContactHeader("Contact", request, resp, &expires)
	tx.Respond(resp)
}

// handleConnectionError 处理连接错误
func (b *B2BUA) handleConnectionError(connError *transport.ConnectionError) {
	logger.Debugf("Handle Connection Lost: Source: %v, Dest: %v, Network: %v", connError.Source, connError.Dest, connError.Net)
	b.registry.HandleConnectionError(connError)
}

// SetLogLevel 设置日志级别
func (b *B2BUA) SetLogLevel(level log.Level) {
	utils.SetLogLevel("B2BUA", level)
}
