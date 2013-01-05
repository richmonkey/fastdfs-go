package main

import "math/rand"
import "hash/crc32"
import "bytes"
import "encoding/binary"
import "encoding/base64"
import "fmt"
import "os"
import "net"
import "strconv"
import "io"
import "syscall"
import "time"
import "log"
import "github.com/kless/goconfig/config"
//godoc -http=:8080
//group1/M00/00/00/wKgBClCyDeKAHnIjAAAABncc3SA6654479
var root_path = ""
var binlog_root_path = ""

type Errno int32
func (e Errno) Error() string {
	return "errno "
}
type OMessage interface {
	Marshal() ([]byte, error)	
}
type IMessage interface {
	Unmarshal([]byte) error	
}

type Message interface {
	IMessage
	OMessage
}

type Header struct{
	pkg_len int64
	cmd int8
	status int8
}
func (this *Header)Marshal() ([]byte, error){
	buffer := new(bytes.Buffer)
	binary.Write(buffer, binary.BigEndian, this.pkg_len)
	buffer.WriteByte(byte(this.cmd))
	buffer.WriteByte(byte(this.status))
	return buffer.Bytes(), nil
}

func (this *Header)Unmarshal(data []byte) error{
	if len(data) != 10 {
		return Errno(0xFF)
	}
	buff := bytes.NewBuffer(data)
	binary.Read(buff, binary.BigEndian, &this.pkg_len)
	cmd, _ := buff.ReadByte()
	status, _ := buff.ReadByte()
	this.cmd = int8(cmd)
	this.status = int8(status)
	return nil
}

type DownloadFileRequest struct {
	file_offset int64
	download_bytes int64
	group_name string
	file_id string
}

func read_cstr(buff io.Reader, length int) (string, error) {
	group_name := make([]byte, length)
	n, err := buff.Read(group_name)
	if err != nil || n != len(group_name) {
		return "", Errno(0xFF)
	}

	for i, v := range(group_name) {
		if v == 0 {
			group_name = group_name[0:i]
			break
		}
	}
	return string(group_name), nil
}
const FDFS_GROUP_NAME_MAX_LEN	= 16

func (this *DownloadFileRequest)Unmarshal(data []byte) error{
	buff := bytes.NewBuffer(data)
	binary.Read(buff, binary.BigEndian, &this.file_offset)
	binary.Read(buff, binary.BigEndian, &this.download_bytes)
	var err error
	this.group_name, err = read_cstr(buff, FDFS_GROUP_NAME_MAX_LEN)
	if err != nil {
		return err
	}
	file_id := data[len(data) - buff.Len():]
	this.file_id = string(file_id)
	return nil
}

type DownloadFileResponse struct{
	file_data []byte
}
func (this *DownloadFileResponse)Marshal() ([]byte, error){
	return this.file_data, nil
}

const FDFS_FILE_EXT_NAME_MAX_LEN = 6
type UploadFileRequest struct{
	store_path_index uint8
	file_length int64
	file_ext string
	file_data []byte
}

func (this *UploadFileRequest)Unmarshal(data []byte) error{
	buff := bytes.NewBuffer(data)
	index, err := buff.ReadByte()
	if err != nil {
		return err
	}
	this.store_path_index = uint8(index)
	err = binary.Read(buff, binary.BigEndian, &this.file_length)
	if err != nil {
		return err
	}
	ext, err := read_cstr(buff, FDFS_FILE_EXT_NAME_MAX_LEN)
	if err != nil {
		return err
	}
	this.file_ext = ext
	this.file_data = data[len(data)-buff.Len():]
	return nil
}

type UploadFileResponse struct{
	group_name string
	file_id string
}

func (this *UploadFileResponse)Marshal() ([]byte, error){
	var buff bytes.Buffer
	buff.WriteString(this.group_name)
	pad_len := FDFS_GROUP_NAME_MAX_LEN - len(this.group_name)
	buff.Write(make([]byte, pad_len))
	buff.WriteString(this.file_id)
	return buff.Bytes(), nil
}

type DeleteFileRequest struct {
	group_name string
	file_id string
}
func (this *DeleteFileRequest)Unmarshal(data []byte) error{
	buff := bytes.NewBuffer(data)
	var err error
	this.group_name, err = read_cstr(buff, FDFS_GROUP_NAME_MAX_LEN)
	if err != nil {
		return err
	}
	this.file_id = string(data[len(data)-buff.Len():])
	return nil
}


type SyncCopyRequest struct {
	file_length int64
	group_name string
	timestamp int32
	file_id string
	file_data []byte
}

func (this *SyncCopyRequest)Unmarshal(data []byte) error{
	var fname_len int64
	buff := bytes.NewBuffer(data)
	err := binary.Read(buff, binary.BigEndian, &fname_len)
	if err != nil {
		return err
	}
	err = binary.Read(buff, binary.BigEndian, &this.file_length)
	if err != nil {
		return err
	}
	err = binary.Read(buff, binary.BigEndian, &this.timestamp)
	if err != nil {
		return err
	}
	
	this.group_name, err = read_cstr(buff, FDFS_GROUP_NAME_MAX_LEN)
	if err != nil {
		return err
	}
	tmp := make([]byte, fname_len)
	n , err := buff.Read(tmp)
	if err != nil {
		return err
	}
	if n != int(fname_len) {
		return Errno(-1)
	}
	this.file_id = string(tmp)
	return nil
}

type SyncDeleteRequest struct {
	timestamp int32
	group_name string
	file_id string
}

func (this *SyncDeleteRequest)Unmarshal(data []byte) error{
	buff := bytes.NewBuffer(data)
	err := binary.Read(buff, binary.BigEndian, &this.timestamp)
	if err != nil {
		return err
	}
	this.group_name, err = read_cstr(buff, FDFS_GROUP_NAME_MAX_LEN)
	if err != nil {
		return err
	}
	file_id := data[len(data) - buff.Len():]
	this.file_id = string(file_id)
	return nil
}

func COMBINE_RAND_FILE_SIZE(file_size int64) int64{
	var r int64 = int64(rand.Int31() & 0x007FFFFF) | int64(0x80000000)
	return r << 32 | int64(file_size)
}

func storage_gen_filename(ip string, file_size int64, crc32 int, fext string, timestamp int) string {
	server_id := []byte(net.ParseIP(ip).To4())
	buffer := new(bytes.Buffer)
	_, err := buffer.Write(server_id)
	if err != nil {
		panic("error")
	}
	err = binary.Write(buffer, binary.BigEndian, int32(timestamp))
	if err != nil {
		panic("error")
	}
	err = binary.Write(buffer, binary.BigEndian, COMBINE_RAND_FILE_SIZE(file_size))
	if err != nil {
		panic("error") 
	}

	err = binary.Write(buffer, binary.BigEndian, int32(crc32))
	if err != nil {
		panic("error")
	}

	encoder := base64.StdEncoding
	encoded := encoder.EncodeToString(buffer.Bytes())
	no_pad_len := (len(buffer.Bytes())*8 + 5)/6
	encoded = encoded[:no_pad_len]
	sub_path_high := 0
	sub_path_low := 0
	filename := fmt.Sprintf("%02X/%02X/%s%s", sub_path_high, sub_path_low, encoded, fext)
	return filename
}

func file_exist(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	return false
}

func storage_get_filename(ip string, store_path_index int, start_time int, 
	file_size int64, crc32 int, fext string) (string, string) {
	for i := 0; i < 10; i++ {
		filename := storage_gen_filename(ip, file_size, crc32, fext, start_time)
		fullname := fmt.Sprintf("%s/data/%s", root_path, filename)
		if !file_exist(filename) {
			return filename, fullname
		}
	}
	return "", ""
}

//"M01/XXXXXX"
const FDFS_STORAGE_STORE_PATH_PREFIX_CHAR  = 'M'
func storage_split_filename(logic_filename string)(filename string, store_path_index int8) {
	i, _ := strconv.ParseInt(logic_filename[1:3], 16, 8)
	store_path_index = int8(i)
	filename = logic_filename[4:]
	return
}

func errno(err error) int{
	fmt.Println("error:", err)
	panic("error")
	e, ok := err.(*os.PathError)
	if !ok {
		return -1
	}
	ee, _ := e.Err.(syscall.Errno)
	dd := uintptr(ee)
	return int(dd)
}

func storage_server_download_file(header *Header, conn net.Conn) bool{
	var data []byte
	result := 0
	buff := make([]byte, header.pkg_len)
	n, err := io.ReadFull(conn, buff)
	if err != nil || n != int(header.pkg_len) {
		return false
	}

	req := &DownloadFileRequest{}
	err = req.Unmarshal(buff)
	if err != nil {
		return false
	}

	//store_path_index
	filename, _ := storage_split_filename(req.file_id)
	filename = fmt.Sprintf("%s/data/%s", root_path, filename)
	fmt.Println(req.file_id, filename)
	file, err := os.Open(filename)
	if err != nil {
		result = errno(err)
		goto Error
	}

	if req.download_bytes == 0 {
		finfo, err := file.Stat()
		if err != nil {
			result = errno(err)
			goto Error
		}
		req.download_bytes = finfo.Size()
	}

	fmt.Println(len(req.group_name), req.group_name, req.file_id, req.download_bytes, req.file_offset)

	data = make([]byte, req.download_bytes)
	n, err = file.ReadAt(data, req.file_offset)
	if err != nil {
		result = errno(err)
		goto Error
	}
	if n != int(req.download_bytes) {
		result = 127
		goto Error
	}
	return send_response(&DownloadFileResponse{data}, conn)
Error:
	return send_result(result, conn)
}

func storage_format_ext_name(ext string) string{
	fext := ""
	ext_len := len(ext)
	pad_len := 0
	if ext_len == 0 {
		pad_len = FDFS_FILE_EXT_NAME_MAX_LEN +1
	} else {
		pad_len = FDFS_FILE_EXT_NAME_MAX_LEN - ext_len
	}
	for i := 0; i < pad_len; i++ {
		fext += string('0' + rand.Intn(10))
	}
	if ext_len > 0 {
		fext += "."
		fext += ext
	}
	return fext
}

func storage_binlog_write(timestamp int, op int8, filename string){
	binlog_path := fmt.Sprintf("%s/data/sync/binlog.000", binlog_root_path)
	file, err := os.OpenFile(binlog_path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		panic("error")
		return
	}
	rec := fmt.Sprintf("%d %c %s\n", timestamp, op, filename)
	_, err = file.Write([]byte(rec))
	if err != nil {
		panic("error")
	}
	file.Close()
	log.Printf(rec)
}

const STORAGE_OP_TYPE_SOURCE_CREATE_FILE  =	'C'
const STORAGE_OP_TYPE_SOURCE_DELETE_FILE  =	'D'
const STORAGE_OP_TYPE_REPLICA_CREATE_FILE =	'c'
const STORAGE_OP_TYPE_REPLICA_DELETE_FILE =	'd'

func storage_upload_file(header *Header, conn net.Conn) bool{
	buff := make([]byte, header.pkg_len)
	n, err := io.ReadFull(conn, buff)
	if err != nil || n != int(header.pkg_len) {
		return false
	}
	req := &UploadFileRequest{}
	err = req.Unmarshal(buff)
	if err != nil {
		return false
	}

	file_ext := storage_format_ext_name(req.file_ext)
	crc32 := crc32.ChecksumIEEE(req.file_data)
	now := time.Now()
//	log.Println(req.store_path_index, req.file_length,  len(req.file_data), file_ext, crc32)

	addr , _ := conn.RemoteAddr().(*net.TCPAddr)
	file_name, full_name := storage_get_filename(addr.IP.String(), int(req.store_path_index), int(now.Unix()), req.file_length, int(crc32), file_ext)
	file, err  := os.OpenFile(full_name, os.O_WRONLY|os.O_EXCL|os.O_CREATE, 0644)
	if err != nil {
		log.Print(err)
		return send_result(errno(err), conn)
	}
	n, err = file.Write(req.file_data)
	if err != nil || n != len(req.file_data) {
		panic("write")
	}

	file_name = fmt.Sprintf("M00/%s", file_name)
	storage_binlog_write(int(now.Unix()), STORAGE_OP_TYPE_SOURCE_CREATE_FILE, file_name)

	resp := &UploadFileResponse{"group1", file_name}
	return send_response(resp, conn)
}

//	STORAGE_OP_TYPE_SOURCE_DELETE_FILE
func storage_server_delete_file(header *Header, conn net.Conn) bool{
	result := 0
	buff := make([]byte, header.pkg_len)
	n, err := io.ReadFull(conn, buff)
	if err != nil || n != int(header.pkg_len) {
		return false
	}
	req := &DeleteFileRequest{}
	err = req.Unmarshal(buff)
	if err != nil {
		return false
	}
	filename, _ := storage_split_filename(req.file_id)
	filename = fmt.Sprintf("%s/data/%s", root_path, filename)
	err = os.Remove(filename)
	if err != nil {
		log.Println("remove file fail")
		result = errno(err)
		goto Exit
	}
	storage_binlog_write(int(time.Now().Unix()), STORAGE_OP_TYPE_SOURCE_DELETE_FILE, req.file_id)
Exit:
	return send_result(result, conn)
}

func storage_sync_copy_file(header *Header, conn net.Conn) bool{
	result := 0
	buff := make([]byte, header.pkg_len)
	n, err := io.ReadFull(conn, buff)
	if err != nil || n != int(header.pkg_len) {
		return false
	}
	req := &SyncCopyRequest{}
	err = req.Unmarshal(buff)
	if err != nil {
		return false
	}
	have_file_content := header.status == 0
	filename, _ := storage_split_filename(req.file_id)
	filename = fmt.Sprintf("%s/data/%s", root_path, filename)
	if have_file_content {
		file, err  := os.OpenFile(filename, os.O_WRONLY|os.O_EXCL|os.O_CREATE, 0644)
		if err != nil {
			fmt.Println(err)
			result = errno(err)
			goto Exit
		}
		n, err = file.Write(req.file_data)
		if err != nil || n != len(req.file_data) {
			panic("write")
		}
		storage_binlog_write(int(req.timestamp), STORAGE_OP_TYPE_REPLICA_CREATE_FILE, req.file_id)
		addr , _ := conn.RemoteAddr().(*net.TCPAddr)
		log.Println("remote addr:", addr.IP.String())
		UpdateSyncSrcTimestamp(addr.IP.String(), int(req.timestamp))
	} else {
		result = int(syscall.EEXIST)
	}

Exit:
	return send_result(result, conn)
}

func storage_sync_delete_file(header *Header, conn net.Conn) bool{
	buff := make([]byte, header.pkg_len)
	n, err := io.ReadFull(conn, buff)
	if err != nil || n != int(header.pkg_len) {
		return false
	}
	req := &SyncDeleteRequest{}
	err = req.Unmarshal(buff)
	if err != nil {
		return false
	}
	filename, _ := storage_split_filename(req.file_id)
	filename = fmt.Sprintf("%s/data/%s", root_path, filename)
	err = os.Remove(filename)
	if err != nil {
		return send_result(errno(err), conn)
	}

	storage_binlog_write(int(req.timestamp), STORAGE_OP_TYPE_REPLICA_DELETE_FILE, req.file_id)

	return send_result(0, conn)
}

func storage_deal_active_test(header *Header, conn net.Conn) bool{
	return send_result(0, conn) 
}

const STORAGE_PROTO_CMD_RESP = 100
func send_result(status int, conn net.Conn) bool {
	return send_response_p(int8(status), nil, conn)
}
func send_response(resp OMessage, conn net.Conn) bool{
	return send_response_p(0, resp, conn)
}
func send_response_p(status int8, resp OMessage, conn net.Conn) bool{
	var data []byte = nil
	var err error 
	if resp != nil {
		data, err = resp.Marshal()
		if err != nil {
			panic("marshal")
		}
	}

	header := &Header{}
	header.cmd = STORAGE_PROTO_CMD_RESP
	header.pkg_len = int64(len(data))
	header.status = status
	buff, err := header.Marshal()
	if err != nil {
		panic("marshal")
	}
	n , err := conn.Write(buff)
	if err != nil {
		return false
	}
	if resp == nil {
		return true
	}
	n, err = conn.Write(data)
	if err != nil || n != len(data) {
		return false
	}
	return true
}

const STORAGE_PROTO_CMD_REPORT_SERVER_ID =	9  
const STORAGE_PROTO_CMD_UPLOAD_FILE	=	11
const STORAGE_PROTO_CMD_DELETE_FILE	=	12
const STORAGE_PROTO_CMD_SET_METADATA	=	13
const STORAGE_PROTO_CMD_DOWNLOAD_FILE	=	14
const STORAGE_PROTO_CMD_GET_METADATA	=	15
const STORAGE_PROTO_CMD_SYNC_CREATE_FILE = 16
const STORAGE_PROTO_CMD_SYNC_DELETE_FILE =	17
const STORAGE_PROTO_CMD_SYNC_UPDATE_FILE =	18
const STORAGE_PROTO_CMD_SYNC_CREATE_LINK =	19
const STORAGE_PROTO_CMD_CREATE_LINK	=	20
const STORAGE_PROTO_CMD_UPLOAD_SLAVE_FILE	= 21
const STORAGE_PROTO_CMD_QUERY_FILE_INFO	= 22
const STORAGE_PROTO_CMD_UPLOAD_APPENDER_FILE=23 
const STORAGE_PROTO_CMD_APPEND_FILE	=	24  
const STORAGE_PROTO_CMD_SYNC_APPEND_FILE	=25
const STORAGE_PROTO_CMD_FETCH_ONE_PATH_BINLOG=	26   

const FDFS_PROTO_CMD_ACTIVE_TEST	=			111 

func handle_request(conn *net.TCPConn) bool {
	buff := make([]byte, 10)
	_, err := io.ReadFull(conn, buff)
	if err != nil {
		return false
	}
	header := &Header{}
	err = header.Unmarshal(buff)
	if err != nil {
		return false
	}
	log.Println("cmd:", header.cmd)
	switch header.cmd {
	case STORAGE_PROTO_CMD_DOWNLOAD_FILE:
		return storage_server_download_file(header, conn)
	case STORAGE_PROTO_CMD_UPLOAD_FILE:
		return storage_upload_file(header, conn)
	case STORAGE_PROTO_CMD_DELETE_FILE:
		return storage_server_delete_file(header, conn)
	case STORAGE_PROTO_CMD_SYNC_CREATE_FILE:
		return storage_sync_copy_file(header, conn)
	case STORAGE_PROTO_CMD_SYNC_DELETE_FILE:
		return storage_sync_delete_file(header, conn)
	case FDFS_PROTO_CMD_ACTIVE_TEST:
		return storage_deal_active_test(header, conn)
	default:
		log.Println("error", header.cmd)
		return false
	}
	return false
}

func handle_client(conn *net.TCPConn) {
	for {
		if !handle_request(conn) {
			break
		}
	}
}

func read_config() {
	if len(os.Args) == 1 { panic("config") }
	cf := os.Args[1]
	conf, err := config.ReadDefault(cf)
	if err != nil { panic("config") }
	binlog_root_path, err = conf.String("DEFAULT", "base_path")
	if err != nil { panic("config") }
	root_path, err = conf.String("DEFAULT", "store_path0")
	if err != nil { panic("config") }
}
func main() {
	if InitShm() == -1 {
		panic("init shm")
	}
	read_config()
	log.SetFlags(log.Lshortfile|log.LstdFlags)
	rand.Seed(time.Now().UnixNano())
	ip := net.ParseIP("0.0.0.0")
	addr := net.TCPAddr{ip, 23000}

	listen, err := net.ListenTCP("tcp", &addr);
	if err != nil {
		fmt.Println("初始化失败", err.Error())
		return
	}
	for {
		client, err := listen.AcceptTCP();
		if err != nil {
			return
		}
		go handle_client(client)
	}
}