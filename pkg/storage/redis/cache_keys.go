package redis

import (
	"strings"
)

type CacheKeys struct {
	prefix string
}

func NewCacheKeys(prefix string) *CacheKeys {
	return &CacheKeys{prefix: prefix}
}

func (k *CacheKeys) BuildKey(components ...string) string {
	allComponents := append([]string{k.prefix}, components...)
	return strings.Join(allComponents, ":")
}
