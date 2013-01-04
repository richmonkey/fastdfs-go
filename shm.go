package main

/*
#cgo CFLAGS: -I/home/houxh/document/FastDFS/storage -I/home/houxh/document/FastDFS/common -I/home/houxh/document/FastDFS/tracker -I/home/houxh/document/FastDFS/client -I/home/houxh/document/FastDFS/storage/fdht_client
#include <stdlib.h>
#include <stdio.h>
#include <sys/ipc.h>
#include <sys/shm.h>
#include <storage_global.h>
#include "shm.c"

*/
import "C"
import "unsafe"
func InitShm() int {
	return int(C.init_shm())
}

func UpdateSyncSrcTimestamp(client_ip string, timestamp int) {
	ip := C.CString(client_ip)
	C.update_timestamp(ip, C.int(timestamp))
	C.free(unsafe.Pointer(ip))
}