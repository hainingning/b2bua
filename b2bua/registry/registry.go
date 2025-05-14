package registry

import (
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/transport"
)

// ContactInstance 表示一个联系实例，包含联系信息、注册过期时间、最后更新时间、来源、用户代理和传输协议。
type ContactInstance struct {
	Contact     *sip.ContactHeader
	RegExpires  uint32
	LastUpdated uint32
	Source      string
	UserAgent   string
	Transport   string
}

// NewContactInstanceForRequest 根据 SIP 请求创建一个新的联系实例。
func NewContactInstanceForRequest(request sip.Request) *ContactInstance {
	expiresHeaders := request.GetHeaders("Expires")
	var expires sip.Expires = 0
	if len(expiresHeaders) > 0 {
		expires = *expiresHeaders[0].(*sip.Expires)
	}

	contacts, _ := request.Contact()
	userAgentHeader := request.GetHeaders("User-Agent")[0].(*sip.UserAgentHeader)

	return &ContactInstance{
		Source:     request.Source(),
		RegExpires: uint32(expires),
		Contact:    contacts.Clone().(*sip.ContactHeader),
		UserAgent:  userAgentHeader.String(),
		Transport:  request.Transport(),
	}
}

// Registry 是 Address-of-Record (AOR) 注册表的接口。
type Registry interface {
	AddAor(aor sip.Uri, instance *ContactInstance) error             // 添加一个 AOR 及其联系实例
	RemoveAor(aor sip.Uri) error                                     // 移除一个 AOR 及其所有联系实例
	AorIsRegistered(aor sip.Uri) bool                                // 检查一个 AOR 是否已注册
	UpdateContact(aor sip.Uri, instance *ContactInstance) error      // 更新一个 AOR 的联系实例
	RemoveContact(aor sip.Uri, instance *ContactInstance) error      // 移除一个 AOR 的特定联系实例
	GetContacts(aor sip.Uri) (*map[string]*ContactInstance, bool)    // 获取一个 AOR 的所有联系实例
	GetAllContacts() map[sip.Uri]map[string]*ContactInstance         // 获取所有 AOR 及其联系实例
	HandleConnectionError(connError *transport.ConnectionError) bool // 处理连接错误
}
