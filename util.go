package nsocket

import "strings"

func resolveNamespace(namespace string) (s string) {
	if namespace == Default {
		return Default
	}
	if namespace == "/" || namespace == "" {
		return Default
	}
	s = strings.TrimPrefix(strings.TrimPrefix(namespace, Default), "/")
	s = strings.TrimSuffix(s, "/")
	s = Default + "/" + s
	return
}
