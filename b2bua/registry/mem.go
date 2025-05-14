package registry

import (
	"fmt"
	"sync"

	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/transport"
)

// MemoryRegistry 是一个基于内存的 Address-of-Record (AOR) 注册表，使用 sync.Mutex 保证并发安全。
type MemoryRegistry struct {
	mutex *sync.Mutex                             // 用于并发控制的互斥锁
	aors  map[sip.Uri]map[string]*ContactInstance // 存储 AOR 和其对应的联系人实例
}

// NewMemoryRegistry 创建一个新的 MemoryRegistry 实例。
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		aors:  make(map[sip.Uri]map[string]*ContactInstance),
		mutex: new(sync.Mutex),
	}
}

// AddAor 添加一个 AOR 和对应的联系人实例到注册表中。
func (mr *MemoryRegistry) AddAor(aor sip.Uri, instance *ContactInstance) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	instances, _ := findInstances(mr.aors, aor)
	if instances != nil {
		(*instances)[instance.Source] = instance // 如果 AOR 已存在，更新联系人实例
		return nil
	}

	mr.aors[aor] = make(map[string]*ContactInstance) // 如果 AOR 不存在，创建新的映射
	mr.aors[aor][instance.Source] = instance         // 添加联系人实例
	return nil
}

// RemoveAor 从注册表中移除指定的 AOR。
func (mr *MemoryRegistry) RemoveAor(aor sip.Uri) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	for key := range mr.aors {
		if key.Equals(aor) {
			delete(mr.aors, key) // 删除匹配的 AOR
			break
		}
	}
	return nil
}

// AorIsRegistered 检查指定的 AOR 是否已注册。
func (mr *MemoryRegistry) AorIsRegistered(aor sip.Uri) bool {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	_, ok := mr.aors[aor]
	return ok
}

// UpdateContact 更新指定 AOR 的联系人实例。
func (mr *MemoryRegistry) UpdateContact(aor sip.Uri, instance *ContactInstance) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	instances, err := findInstances(mr.aors, aor)
	if err != nil {
		return err
	}

	(*instances)[instance.Source] = instance // 更新联系人实例
	return nil
}

// RemoveContact 从指定 AOR 中移除一个联系人实例。
func (mr *MemoryRegistry) RemoveContact(aor sip.Uri, instance *ContactInstance) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	instances, err := findInstances(mr.aors, aor)
	if instances != nil {
		delete(*instances, instance.Source) // 删除联系人实例

		// 如果 AOR 的联系人实例为空，移除整个 AOR
		if len(*instances) == 0 {
			for key := range mr.aors {
				if key.Equals(aor) {
					delete(mr.aors, key)
					break
				}
			}
		}
		return nil
	}
	return err
}

// HandleConnectionError 处理连接错误，移除与错误源相关的联系人实例。
func (mr *MemoryRegistry) HandleConnectionError(connError *transport.ConnectionError) bool {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	result := false
	for aor, cis := range mr.aors {
		for source := range cis {
			if source == connError.Source {
				delete(cis, source) // 删除与错误源相关的联系人实例
				result = true
				break
			}
		}

		// 如果 AOR 的联系人实例为空，移除整个 AOR
		if len(cis) == 0 {
			delete(mr.aors, aor)
			break
		}
	}
	return result
}

// GetContacts 获取指定 AOR 的所有联系人实例。
func (mr *MemoryRegistry) GetContacts(aor sip.Uri) (*map[string]*ContactInstance, bool) {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	instance, err := findInstances(mr.aors, aor)
	if err != nil {
		return nil, false
	}
	return instance, true
}

// GetAllContacts 获取注册表中所有 AOR 及其联系人实例。
func (mr *MemoryRegistry) GetAllContacts() map[sip.Uri]map[string]*ContactInstance {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	return mr.aors
}

// findInstances 根据 AOR 查找对应的联系人实例映射。
func findInstances(aors map[sip.Uri]map[string]*ContactInstance, aor sip.Uri) (*map[string]*ContactInstance, error) {
	for key, instances := range aors {
		if key.User() == aor.User() { // 根据用户部分匹配 AOR
			return &instances, nil
		}
	}
	return nil, fmt.Errorf("not found instances for %v", aor)
}
