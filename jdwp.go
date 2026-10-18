package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	csVirtualMachine     = 1
	csReferenceType      = 2
	csClassType          = 3
	csInterfaceType      = 5
	csMethod             = 6
	csObjectReference    = 9
	csStringReference    = 10
	csThreadReference    = 11
	csArrayReference     = 13
	csEventRequest       = 15
	csStackFrame         = 16
	csClassObjectRefType = 17
	csEvent              = 64
)

const (
	cmdVMVersion            = 1
	cmdVMClassesBySignature = 2
	cmdVMAllClasses         = 3
	cmdVMAllThreads         = 4
	cmdVMDispose            = 6
	cmdVMIDSizes            = 7
	cmdVMSuspend            = 8
	cmdVMResume             = 9
	cmdVMExit               = 10
	cmdVMCreateString       = 11
)

const (
	cmdRefSignature    = 1
	cmdRefFields       = 4
	cmdRefMethods      = 5
	cmdRefStaticValues = 6
	cmdRefSourceFile   = 7
)

const (
	cmdClassSuperclass = 1
	cmdClassSetValues  = 2
	cmdClassInvoke     = 3
)

const (
	cmdMethodLineTable     = 1
	cmdMethodVariableTable = 2
)

const (
	cmdObjRefType   = 1
	cmdObjGetValues = 2
	cmdObjSetValues = 3
	cmdObjInvoke    = 6
)

const (
	cmdStringValue = 1
)

const (
	cmdThreadName       = 1
	cmdThreadSuspend    = 2
	cmdThreadResume     = 3
	cmdThreadStatus     = 4
	cmdThreadFrames     = 6
	cmdThreadFrameCount = 7
)

const (
	cmdArrayLength    = 1
	cmdArrayGetValues = 2
)

const (
	cmdEventSet   = 1
	cmdEventClear = 2
)

const (
	cmdFrameGetValues  = 1
	cmdFrameSetValues  = 2
	cmdFrameThisObject = 3
)

const (
	cmdCompositeEvent = 100
)

const (
	tagArray    = 91
	tagByte     = 66
	tagChar     = 67
	tagObject   = 76
	tagFloat    = 70
	tagDouble   = 68
	tagInt      = 73
	tagLong     = 74
	tagShort    = 83
	tagVoid     = 86
	tagBoolean  = 90
	tagString   = 115
	tagThread   = 116
	tagThreadGp = 103
	tagLoader   = 108
	tagClassObj = 99
)

const (
	evSingleStep  = 1
	evBreakpoint  = 2
	evThreadStart = 6
	evThreadDeath = 7
	evClassPrep   = 8
	evVMStart     = 90
	evVMDeath     = 99
)

const (
	suspendNone   = 0
	suspendThread = 1
	suspendAll    = 2
)

const (
	modKindCount               = 1
	modKindThreadOnly          = 3
	modKindClassOnly           = 4
	modKindClassMatch          = 5
	modKindClassExclude        = 6
	modKindLocationOnly        = 7
	modKindExceptionOnly       = 8
	modKindFieldOnly           = 9
	modKindStep                = 10
	modKindInstanceOnly        = 11
	modKindSourceName          = 12
	modKindPlatformThreadsOnly = 13
)

const (
	stepInto = 0
	stepOver = 1
	stepOut  = 2
)

const (
	stepSizeLine = 1
)

const (
	errNone              = 0
	errInvalidThread     = 10
	errThreadNotSusp     = 13
	errInvalidObject     = 20
	errInvalidClass      = 21
	errClassNotPrepared  = 22
	errInvalidMethodID   = 23
	errInvalidLocation   = 24
	errInvalidFieldID    = 25
	errInvalidFrameID    = 30
	errNoMoreFrames      = 31
	errOpaqueFrame       = 32
	errNotCurrentFrame   = 33
	errTypeMismatch      = 34
	errInvalidSlot       = 35
	errNotFound          = 41
	errNotImplemented    = 99
	errAbsentInformation = 101
	errInvalidEventType  = 102
	errIllegalArgument   = 103
	errVMDead            = 112
	errInternal          = 113
	errInvalidTag        = 500
)

const (
	statusZombie   = 0
	statusRunning  = 1
	statusSleeping = 2
	statusMonitor  = 3
	statusWait     = 4
)

var jdwpErrorNames = map[int]string{
	errInvalidThread:     "线程无效",
	errThreadNotSusp:     "线程未被挂起",
	errInvalidObject:     "对象无效",
	errInvalidClass:      "类无效",
	errClassNotPrepared:  "类尚未初始化",
	errInvalidMethodID:   "方法无效",
	errInvalidLocation:   "位置无效",
	errInvalidFieldID:    "字段无效",
	errInvalidFrameID:    "栈帧无效",
	errNoMoreFrames:      "没有更多栈帧",
	errOpaqueFrame:       "栈帧信息不可用",
	errNotCurrentFrame:   "只能操作当前栈帧",
	errTypeMismatch:      "类型不匹配",
	errInvalidSlot:       "局部变量槽位无效",
	errNotFound:          "未找到目标",
	errNotImplemented:    "虚拟机不支持该功能",
	errAbsentInformation: "缺少调试信息",
	errInvalidEventType:  "事件类型无效",
	errIllegalArgument:   "参数非法",
	errVMDead:            "虚拟机已退出",
	errInternal:          "虚拟机内部错误",
	errInvalidTag:        "类型标签无效",
}

func jdwpErrorText(code int) string {
	if name, ok := jdwpErrorNames[code]; ok {
		return name
	}
	return "JDWP 错误 " + strconv.Itoa(code)
}

type jdwpSizes struct {
	fieldID, methodID, objectID, refTypeID, frameID int
}

type jdwpLoc struct {
	tag    byte
	class  int64
	method int64
	index  int64
}

type jdwpValue struct {
	tag byte
	i   int64
	f   float64
	obj int64
}

func (v jdwpValue) isObject() bool {
	switch v.tag {
	case tagObject, tagString, tagThread, tagThreadGp, tagLoader, tagClassObj, tagArray:
		return true
	}
	return false
}

func (v jdwpValue) primitive() string {
	switch v.tag {
	case tagBoolean:
		if v.i != 0 {
			return "true"
		}
		return "false"
	case tagByte:
		return strconv.FormatInt(v.i, 10)
	case tagChar:
		return "'" + string(rune(v.i)) + "'"
	case tagShort:
		return strconv.FormatInt(v.i, 10)
	case tagInt:
		return strconv.FormatInt(v.i, 10)
	case tagLong:
		return strconv.FormatInt(v.i, 10) + "L"
	case tagFloat:
		return strconv.FormatFloat(v.f, 'g', -1, 32) + "f"
	case tagDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case tagVoid:
		return "void"
	}
	return ""
}

type jdwpMethod struct {
	id     int64
	name   string
	sig    string
	static bool
}

type jdwpField struct {
	id     int64
	name   string
	sig    string
	static bool
}

type jdwpVarSlot struct {
	name string
	sig  string
	slot int
}

type jdwpFrame struct {
	id  int64
	loc jdwpLoc
}

type jdwpEvent struct {
	kind      byte
	requestID int
	thread    int64
	loc       jdwpLoc
	typeID    int64
	tag       byte
	signature string
}

type jdwpEventBatch struct {
	policy byte
	events []jdwpEvent
}

type jdwpPacket struct {
	id      uint32
	flags   byte
	errCode uint16
	cmdSet  byte
	cmd     byte
	data    []byte
}

type jdwpConn struct {
	conn  net.Conn
	r     *bufio.Reader
	w     *bufio.Writer
	mu    sync.Mutex
	id    uint32
	wait  map[uint32]chan *jdwpPacket
	sizes jdwpSizes

	events   chan jdwpEventBatch
	dead     chan struct{}
	onEvents func(jdwpEventBatch)
	closeOne sync.Once
	deadErr  error
	deadMu   sync.Mutex
}

func dialJDWP(port int, timeout time.Duration, onEvents func(jdwpEventBatch)) (*jdwpConn, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			j := &jdwpConn{
				conn:     c,
				r:        bufio.NewReader(c),
				w:        bufio.NewWriter(c),
				wait:     map[uint32]chan *jdwpPacket{},
				events:   make(chan jdwpEventBatch, 256),
				dead:     make(chan struct{}),
				onEvents: onEvents,
			}
			if err := j.handshake(); err != nil {
				c.Close()
				return nil, err
			}
			go j.readLoop()
			go j.eventLoop()
			return j, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (j *jdwpConn) handshake() error {
	if _, err := j.conn.Write([]byte("JDWP-Handshake")); err != nil {
		return err
	}
	got := make([]byte, 14)
	if _, err := io.ReadFull(j.conn, got); err != nil {
		return err
	}
	if string(got) != "JDWP-Handshake" {
		return errors.New("JDWP 握手失败")
	}
	return nil
}

func (j *jdwpConn) fail(err error) {
	j.deadMu.Lock()
	if j.deadErr == nil {
		j.deadErr = err
	}
	j.deadMu.Unlock()
	j.closeOne.Do(func() {
		close(j.dead)
		j.conn.Close()
	})
}

func (j *jdwpConn) close() {
	j.fail(errors.New("连接已关闭"))
}

func (j *jdwpConn) failure() error {
	j.deadMu.Lock()
	defer j.deadMu.Unlock()
	if j.deadErr != nil {
		return j.deadErr
	}
	return errors.New("连接已关闭")
}

func (j *jdwpConn) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			j.fail(fmt.Errorf("协议异常：%v", r))
		}
	}()
	for {
		var hdr [11]byte
		if _, err := io.ReadFull(j.r, hdr[:]); err != nil {
			j.fail(err)
			return
		}
		length := binary.BigEndian.Uint32(hdr[0:4])
		if length < 11 {
			j.fail(errors.New("JDWP 数据包长度非法"))
			return
		}
		data := make([]byte, length-11)
		if _, err := io.ReadFull(j.r, data); err != nil {
			j.fail(err)
			return
		}
		p := &jdwpPacket{id: binary.BigEndian.Uint32(hdr[4:8]), flags: hdr[8], data: data}
		if p.flags&0x80 != 0 {
			p.errCode = binary.BigEndian.Uint16(hdr[9:11])
			j.mu.Lock()
			ch := j.wait[p.id]
			delete(j.wait, p.id)
			j.mu.Unlock()
			if ch != nil {
				ch <- p
			}
			continue
		}
		p.cmdSet, p.cmd = hdr[9], hdr[10]
		if p.cmdSet == csEvent && p.cmd == cmdCompositeEvent {
			if batch, ok := j.parseEvents(data); ok {
				select {
				case j.events <- batch:
				case <-j.dead:
					return
				}
			}
		}
	}
}

func (j *jdwpConn) eventLoop() {
	for {
		select {
		case batch := <-j.events:
			if j.onEvents != nil {
				j.onEvents(batch)
			}
		case <-j.dead:
			return
		}
	}
}

func (j *jdwpConn) parseEvents(data []byte) (jdwpEventBatch, bool) {
	r := &jdwpReader{b: data, sizes: &j.sizes}
	batch := jdwpEventBatch{policy: r.byte()}
	n := int(r.int32())
	if r.err {
		return jdwpEventBatch{}, false
	}
	for range n {
		e := jdwpEvent{kind: r.byte()}
		e.requestID = int(r.int32())
		switch e.kind {
		case evVMStart, evThreadStart, evThreadDeath:
			e.thread = r.id(j.sizes.objectID)
		case evSingleStep, evBreakpoint:
			e.thread = r.id(j.sizes.objectID)
			e.loc = r.loc()
		case evClassPrep:
			e.thread = r.id(j.sizes.objectID)
			e.tag = r.byte()
			e.typeID = r.id(j.sizes.refTypeID)
			e.signature = r.str()
		}
		batch.events = append(batch.events, e)
	}
	if r.err {
		return jdwpEventBatch{}, false
	}
	return batch, true
}

func (j *jdwpConn) command(set, cmd byte, out []byte) (*jdwpPacket, error) {
	j.mu.Lock()
	if j.id == math.MaxUint32 {
		j.id = 0
	}
	j.id++
	id := j.id
	ch := make(chan *jdwpPacket, 1)
	j.wait[id] = ch
	var hdr [11]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(11+len(out)))
	binary.BigEndian.PutUint32(hdr[4:8], id)
	hdr[9], hdr[10] = set, cmd
	j.w.Write(hdr[:])
	j.w.Write(out)
	err := j.w.Flush()
	j.mu.Unlock()
	if err != nil {
		j.mu.Lock()
		delete(j.wait, id)
		j.mu.Unlock()
		return nil, err
	}
	select {
	case p := <-ch:
		if p.errCode != errNone {
			return p, errors.New(jdwpErrorText(int(p.errCode)))
		}
		return p, nil
	case <-j.dead:
		return nil, j.failure()
	case <-time.After(60 * time.Second):
		j.mu.Lock()
		delete(j.wait, id)
		j.mu.Unlock()
		return nil, errors.New("JDWP 命令超时")
	}
}

func (j *jdwpConn) replyData(set, cmd byte, out []byte) ([]byte, error) {
	p, err := j.command(set, cmd, out)
	if err != nil {
		return nil, err
	}
	return p.data, nil
}

func (j *jdwpConn) errorCode(set, cmd byte, out []byte) int {
	p, err := j.command(set, cmd, out)
	if err != nil {
		return errVMDead
	}
	return int(p.errCode)
}

func (j *jdwpConn) version() (string, error) {
	p, err := j.replyData(csVirtualMachine, cmdVMVersion, nil)
	if err != nil {
		return "", err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	desc := r.str()
	r.int32()
	r.int32()
	return desc, nil
}

func (j *jdwpConn) idSizes() error {
	p, err := j.replyData(csVirtualMachine, cmdVMIDSizes, nil)
	if err != nil {
		return err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	j.sizes = jdwpSizes{
		fieldID:   int(r.int32()),
		methodID:  int(r.int32()),
		objectID:  int(r.int32()),
		refTypeID: int(r.int32()),
		frameID:   int(r.int32()),
	}
	if r.err || j.sizes.objectID <= 0 || j.sizes.refTypeID <= 0 {
		return errors.New("无法读取 JDWP 标识尺寸")
	}
	return nil
}

func (j *jdwpConn) allClasses() (map[string]int64, error) {
	p, err := j.replyData(csVirtualMachine, cmdVMAllClasses, nil)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out := map[string]int64{}
	for range n {
		r.byte()
		id := r.id(j.sizes.refTypeID)
		sig := r.str()
		r.int32()
		out[sig] = id
	}
	if r.err {
		return nil, errors.New("解析类列表失败")
	}
	return out, nil
}

func (j *jdwpConn) allThreads() ([]int64, error) {
	p, err := j.replyData(csVirtualMachine, cmdVMAllThreads, nil)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out := make([]int64, 0, n)
	for range n {
		out = append(out, r.id(j.sizes.objectID))
	}
	if r.err {
		return nil, errors.New("解析线程列表失败")
	}
	return out, nil
}

func (j *jdwpConn) threadName(id int64) (string, error) {
	out := writeID(nil, id, j.sizes.objectID)
	p, err := j.replyData(csThreadReference, cmdThreadName, out)
	if err != nil {
		return "", err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.str(), nil
}

func (j *jdwpConn) threadStatus(id int64) int {
	out := writeID(nil, id, j.sizes.objectID)
	p, err := j.replyData(csThreadReference, cmdThreadStatus, out)
	if err != nil {
		return statusZombie
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return int(r.int32())
}

func (j *jdwpConn) frames(thread int64) ([]jdwpFrame, error) {
	out := writeID(nil, thread, j.sizes.objectID)
	out = writeInt32(out, 0)
	out = writeInt32(out, -1)
	p, err := j.replyData(csThreadReference, cmdThreadFrames, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out2 := make([]jdwpFrame, 0, n)
	for range n {
		f := jdwpFrame{id: r.id(j.sizes.frameID)}
		f.loc = r.loc()
		out2 = append(out2, f)
	}
	if r.err {
		return nil, errors.New("解析调用栈失败")
	}
	return out2, nil
}

func (j *jdwpConn) signature(typeID int64) (string, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	p, err := j.replyData(csReferenceType, cmdRefSignature, out)
	if err != nil {
		return "", err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.str(), nil
}

func (j *jdwpConn) sourceFile(typeID int64) (string, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	p, err := j.replyData(csReferenceType, cmdRefSourceFile, out)
	if err != nil {
		return "", err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.str(), nil
}

func (j *jdwpConn) methods(typeID int64) ([]jdwpMethod, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	p, err := j.replyData(csReferenceType, cmdRefMethods, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out2 := make([]jdwpMethod, 0, n)
	for range n {
		m := jdwpMethod{id: r.id(j.sizes.methodID)}
		m.name = r.str()
		m.sig = r.str()
		m.static = r.int32()&0x0008 != 0
		out2 = append(out2, m)
	}
	if r.err {
		return nil, errors.New("解析方法列表失败")
	}
	return out2, nil
}

func (j *jdwpConn) fields(typeID int64) ([]jdwpField, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	p, err := j.replyData(csReferenceType, cmdRefFields, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out2 := make([]jdwpField, 0, n)
	for range n {
		f := jdwpField{id: r.id(j.sizes.fieldID)}
		f.name = r.str()
		f.sig = r.str()
		f.static = r.int32()&0x0008 != 0
		out2 = append(out2, f)
	}
	if r.err {
		return nil, errors.New("解析字段列表失败")
	}
	return out2, nil
}

func (j *jdwpConn) superclass(typeID int64) int64 {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	p, err := j.replyData(csClassType, cmdClassSuperclass, out)
	if err != nil {
		return 0
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.id(j.sizes.refTypeID)
}

func (j *jdwpConn) staticValues(typeID int64, fieldIDs []int64) ([]jdwpValue, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	out = writeInt32(out, int32(len(fieldIDs)))
	for _, id := range fieldIDs {
		out = writeID(out, id, j.sizes.fieldID)
	}
	p, err := j.replyData(csReferenceType, cmdRefStaticValues, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	values := make([]jdwpValue, 0, n)
	for range n {
		values = append(values, r.value())
	}
	if r.err {
		return nil, errors.New("解析静态字段失败")
	}
	return values, nil
}

func (j *jdwpConn) objectType(objID int64) int64 {
	out := writeID(nil, objID, j.sizes.objectID)
	p, err := j.replyData(csObjectReference, cmdObjRefType, out)
	if err != nil {
		return 0
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	r.byte()
	return r.id(j.sizes.refTypeID)
}

func (j *jdwpConn) fieldValues(objID int64, fieldIDs []int64) ([]jdwpValue, error) {
	out := writeID(nil, objID, j.sizes.objectID)
	out = writeInt32(out, int32(len(fieldIDs)))
	for _, id := range fieldIDs {
		out = writeID(out, id, j.sizes.fieldID)
	}
	p, err := j.replyData(csObjectReference, cmdObjGetValues, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	values := make([]jdwpValue, 0, n)
	for range n {
		values = append(values, r.value())
	}
	if r.err {
		return nil, errors.New("解析字段值失败")
	}
	return values, nil
}

func (j *jdwpConn) arrayLength(objID int64) int {
	out := writeID(nil, objID, j.sizes.objectID)
	p, err := j.replyData(csArrayReference, cmdArrayLength, out)
	if err != nil {
		return -1
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return int(r.int32())
}

func (j *jdwpConn) arrayValues(objID int64, first, count int, elemSig string) ([]jdwpValue, error) {
	out := writeID(nil, objID, j.sizes.objectID)
	out = writeInt32(out, int32(first))
	out = writeInt32(out, int32(count))
	p, err := j.replyData(csArrayReference, cmdArrayGetValues, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	ref := len(elemSig) > 0 && (elemSig[0] == 'L' || elemSig[0] == '[')
	values := make([]jdwpValue, 0, count)
	for range count {
		if ref {
			values = append(values, r.tagged())
		} else {
			values = append(values, r.valueTagged(tagFromSig(elemSig)))
		}
	}
	if r.err {
		return nil, errors.New("解析数组元素失败")
	}
	return values, nil
}

func (j *jdwpConn) stringValue(objID int64) (string, error) {
	out := writeID(nil, objID, j.sizes.objectID)
	p, err := j.replyData(csStringReference, cmdStringValue, out)
	if err != nil {
		return "", err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.str(), nil
}

func (j *jdwpConn) lineTable(typeID, methodID int64) (int64, int64, [][2]int64, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	out = writeID(out, methodID, j.sizes.methodID)
	p, err := j.replyData(csMethod, cmdMethodLineTable, out)
	if err != nil {
		return 0, 0, nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	start, end := r.int64(), r.int64()
	n := int(r.int32())
	lines := make([][2]int64, 0, n)
	for range n {
		idx := r.int64()
		num := int64(r.int32())
		lines = append(lines, [2]int64{idx, num})
	}
	if r.err {
		return 0, 0, nil, errors.New("解析行号表失败")
	}
	return start, end, lines, nil
}

func (j *jdwpConn) variableTable(typeID, methodID int64) ([]jdwpVarSlot, error) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	out = writeID(out, methodID, j.sizes.methodID)
	p, err := j.replyData(csMethod, cmdMethodVariableTable, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	r.int32()
	n := int(r.int32())
	slots := make([]jdwpVarSlot, 0, n)
	for range n {
		r.int64()
		name := r.str()
		sig := r.str()
		r.int32()
		slot := int(r.int32())
		slots = append(slots, jdwpVarSlot{name: name, sig: sig, slot: slot})
	}
	if r.err {
		return nil, errors.New("解析局部变量表失败")
	}
	return slots, nil
}

func (j *jdwpConn) localValues(thread, frame int64, slots []int, tags []byte) ([]jdwpValue, error) {
	out := writeID(nil, thread, j.sizes.objectID)
	out = writeID(out, frame, j.sizes.frameID)
	out = writeInt32(out, int32(len(slots)))
	for i, s := range slots {
		out = writeInt32(out, int32(s))
		tag := byte(tagObject)
		if i < len(tags) {
			tag = tags[i]
		}
		out = append(out, tag)
	}
	p, err := j.replyData(csStackFrame, cmdFrameGetValues, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	values := make([]jdwpValue, 0, n)
	for range n {
		values = append(values, r.value())
	}
	if r.err {
		return nil, errors.New("解析局部变量失败")
	}
	return values, nil
}

func (j *jdwpConn) setLocalValue(thread, frame int64, slot int, v jdwpValue) error {
	out := writeID(nil, thread, j.sizes.objectID)
	out = writeID(out, frame, j.sizes.frameID)
	out = writeInt32(out, 1)
	out = writeInt32(out, int32(slot))
	out = writeValue(out, v, j.sizes.objectID)
	_, err := j.replyData(csStackFrame, cmdFrameSetValues, out)
	return err
}

func (j *jdwpConn) setFieldValue(objID, fieldID int64, v jdwpValue) error {
	out := writeID(nil, objID, j.sizes.objectID)
	out = writeInt32(out, 1)
	out = writeID(out, fieldID, j.sizes.fieldID)
	out = writeValue(out, v, j.sizes.objectID)
	_, err := j.replyData(csObjectReference, cmdObjSetValues, out)
	return err
}

func (j *jdwpConn) thisObject(thread, frame int64) (jdwpValue, error) {
	out := writeID(nil, thread, j.sizes.objectID)
	out = writeID(out, frame, j.sizes.frameID)
	p, err := j.replyData(csStackFrame, cmdFrameThisObject, out)
	if err != nil {
		return jdwpValue{}, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.tagged(), nil
}

func (j *jdwpConn) classesBySignature(sig string) ([]int64, error) {
	out := writeString(nil, sig)
	p, err := j.replyData(csVirtualMachine, cmdVMClassesBySignature, out)
	if err != nil {
		return nil, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	n := int(r.int32())
	out2 := make([]int64, 0, n)
	for range n {
		r.byte()
		out2 = append(out2, r.id(j.sizes.refTypeID))
		r.int32()
	}
	if r.err {
		return nil, errors.New("解析类查找结果失败")
	}
	return out2, nil
}

func (j *jdwpConn) createString(s string) (int64, error) {
	p, err := j.replyData(csVirtualMachine, cmdVMCreateString, writeString(nil, s))
	if err != nil {
		return 0, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	return r.id(j.sizes.objectID), nil
}

func (j *jdwpConn) invokeMethod(objID, thread, typeID, methodID int64, args []jdwpValue) (jdwpValue, int64) {
	out := writeID(nil, objID, j.sizes.objectID)
	out = writeID(out, thread, j.sizes.objectID)
	out = writeID(out, typeID, j.sizes.refTypeID)
	out = writeID(out, methodID, j.sizes.methodID)
	out = writeInt32(out, int32(len(args)))
	for _, a := range args {
		out = writeValue(out, a, j.sizes.objectID)
	}
	out = writeInt32(out, 0)
	p, err := j.replyData(csObjectReference, cmdObjInvoke, out)
	if err != nil {
		return jdwpValue{tag: tagVoid}, 0
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	ret := r.value()
	exc := r.tagged()
	return ret, exc.obj
}

func (j *jdwpConn) invokeStatic(thread, typeID, methodID int64, args []jdwpValue) (jdwpValue, int64) {
	out := writeID(nil, typeID, j.sizes.refTypeID)
	out = writeID(out, thread, j.sizes.objectID)
	out = writeID(out, methodID, j.sizes.methodID)
	out = writeInt32(out, int32(len(args)))
	for _, a := range args {
		out = writeValue(out, a, j.sizes.objectID)
	}
	out = writeInt32(out, 0)
	p, err := j.replyData(csClassType, cmdClassInvoke, out)
	if err != nil {
		return jdwpValue{tag: tagVoid}, 0
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	ret := r.value()
	exc := r.tagged()
	return ret, exc.obj
}

func (j *jdwpConn) suspendVM() error {
	_, err := j.replyData(csVirtualMachine, cmdVMSuspend, nil)
	return err
}

func (j *jdwpConn) resumeVM() error {
	_, err := j.replyData(csVirtualMachine, cmdVMResume, nil)
	return err
}

func (j *jdwpConn) resumeThread(id int64) error {
	_, err := j.replyData(csThreadReference, cmdThreadResume, writeID(nil, id, j.sizes.objectID))
	return err
}

func (j *jdwpConn) setEvent(kind, policy byte, mods ...func(out []byte) []byte) (int, error) {
	out := []byte{kind, policy}
	out = writeInt32(out, int32(len(mods)))
	for _, m := range mods {
		out = m(out)
	}
	p, err := j.replyData(csEventRequest, cmdEventSet, out)
	if err != nil {
		return 0, err
	}
	r := &jdwpReader{b: p, sizes: &j.sizes}
	id := int(r.int32())
	return id, nil
}

func (j *jdwpConn) clearEvent(kind byte, requestID int) error {
	out := []byte{kind}
	out = writeInt32(out, int32(requestID))
	_, err := j.replyData(csEventRequest, cmdEventClear, out)
	return err
}

func modLocationOnly(sizes jdwpSizes, loc jdwpLoc) func([]byte) []byte {
	return func(out []byte) []byte {
		out = append(out, modKindLocationOnly)
		out = append(out, loc.tag)
		out = writeID(out, loc.class, sizes.refTypeID)
		out = writeID(out, loc.method, sizes.methodID)
		return writeInt64(out, loc.index)
	}
}

func modClassMatch(pattern string) func([]byte) []byte {
	return func(out []byte) []byte {
		out = append(out, modKindClassMatch)
		return writeString(out, pattern)
	}
}

func modStep(sizes jdwpSizes, thread int64, size, depth int32) func([]byte) []byte {
	return func(out []byte) []byte {
		out = append(out, modKindStep)
		out = writeID(out, thread, sizes.objectID)
		out = writeInt32(out, size)
		return writeInt32(out, depth)
	}
}

func writeInt64(out []byte, v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return append(out, b[:]...)
}

func writeID(out []byte, id int64, size int) []byte {
	if size <= 0 {
		size = 8
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(id))
	return append(out, b[8-size:]...)
}

func writeInt32(out []byte, v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return append(out, b[:]...)
}

func writeString(out []byte, s string) []byte {
	out = writeInt32(out, int32(len(s)))
	return append(out, s...)
}

func writeValue(out []byte, v jdwpValue, objSize int) []byte {
	out = append(out, v.tag)
	switch v.tag {
	case tagByte, tagBoolean:
		out = append(out, byte(v.i))
	case tagChar, tagShort:
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(v.i))
		out = append(out, b[:]...)
	case tagInt, tagFloat:
		var b [4]byte
		if v.tag == tagFloat {
			binary.BigEndian.PutUint32(b[:], math.Float32bits(float32(v.f)))
		} else {
			binary.BigEndian.PutUint32(b[:], uint32(v.i))
		}
		out = append(out, b[:]...)
	case tagLong, tagDouble:
		var b [8]byte
		if v.tag == tagDouble {
			binary.BigEndian.PutUint64(b[:], math.Float64bits(v.f))
		} else {
			binary.BigEndian.PutUint64(b[:], uint64(v.i))
		}
		out = append(out, b[:]...)
	default:
		out = writeID(out, v.obj, objSize)
	}
	return out
}

type jdwpReader struct {
	b     []byte
	pos   int
	err   bool
	sizes *jdwpSizes
}

func (r *jdwpReader) fail() { r.err = true }

func (r *jdwpReader) byte() byte {
	if r.pos+1 > len(r.b) {
		r.fail()
		return 0
	}
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *jdwpReader) int16() int16 {
	if r.pos+2 > len(r.b) {
		r.fail()
		return 0
	}
	v := int16(binary.BigEndian.Uint16(r.b[r.pos:]))
	r.pos += 2
	return v
}

func (r *jdwpReader) int32() int32 {
	if r.pos+4 > len(r.b) {
		r.fail()
		return 0
	}
	v := int32(binary.BigEndian.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v
}

func (r *jdwpReader) int64() int64 {
	if r.pos+8 > len(r.b) {
		r.fail()
		return 0
	}
	v := int64(binary.BigEndian.Uint64(r.b[r.pos:]))
	r.pos += 8
	return v
}

func (r *jdwpReader) str() string {
	n := int(r.int32())
	if n < 0 || r.pos+n > len(r.b) {
		r.fail()
		return ""
	}
	s := string(r.b[r.pos : r.pos+n])
	r.pos += n
	return s
}

func (r *jdwpReader) id(size int) int64 {
	if size <= 0 {
		size = 8
	}
	if r.pos+size > len(r.b) {
		r.fail()
		return 0
	}
	var b [8]byte
	copy(b[8-size:], r.b[r.pos:r.pos+size])
	r.pos += size
	return int64(binary.BigEndian.Uint64(b[:]))
}

func (r *jdwpReader) tagged() jdwpValue {
	tag := r.byte()
	return r.valueTagged(tag)
}

func (r *jdwpReader) value() jdwpValue {
	tag := r.byte()
	return r.valueTagged(tag)
}

func (r *jdwpReader) valueTagged(tag byte) jdwpValue {
	switch tag {
	case tagByte:
		return jdwpValue{tag: tag, i: int64(int8(r.byte()))}
	case tagBoolean:
		b := r.byte()
		v := int64(0)
		if b != 0 {
			v = 1
		}
		return jdwpValue{tag: tag, i: v}
	case tagChar, tagShort:
		return jdwpValue{tag: tag, i: int64(r.int16())}
	case tagInt:
		return jdwpValue{tag: tag, i: int64(r.int32())}
	case tagLong:
		return jdwpValue{tag: tag, i: r.int64()}
	case tagFloat:
		bits := uint32(r.int32())
		return jdwpValue{tag: tag, f: float64(math.Float32frombits(bits))}
	case tagDouble:
		bits := uint64(r.int64())
		return jdwpValue{tag: tag, f: math.Float64frombits(bits)}
	case tagVoid:
		return jdwpValue{tag: tag}
	default:
		return jdwpValue{tag: tag, obj: r.id(r.sizes.objectID)}
	}
}

func (r *jdwpReader) loc() jdwpLoc {
	l := jdwpLoc{tag: r.byte()}
	l.class = r.id(r.sizes.refTypeID)
	l.method = r.id(r.sizes.methodID)
	l.index = r.int64()
	return l
}
