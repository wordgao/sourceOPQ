package OPQBot

import (
	"encoding/json"
	"errors"
	"github.com/asmcos/requests"
	"github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BotManager struct {
	QQ       int64
	SendChan chan SendMsgPack
	Running  bool
	OPQUrl   string
	onEvent  map[string]reflect.Value
	locker   sync.RWMutex
}

func NewBotManager(QQ int64, OPQUrl string) BotManager {
	return BotManager{QQ: QQ, OPQUrl: OPQUrl, SendChan: make(chan SendMsgPack, 1024), onEvent: make(map[string]reflect.Value), locker: sync.RWMutex{}}
}

// 开始连接
func (b *BotManager) Start() error {
	b.Running = true
	go b.receiveSendPack()
	c, err := gosocketio.Dial(strings.ReplaceAll(b.OPQUrl, "http://", "ws://")+"/socket.io/?EIO=3&transport=websocket", transport.GetDefaultWebsocketTransport())
	if err != nil {
		return err
	}
	_ = c.On(gosocketio.OnConnection, func(h *gosocketio.Channel) {
		log.Println("连接成功！")
	})
	_ = c.On(gosocketio.OnDisconnection, func(h *gosocketio.Channel) {
		log.Println("连接断开！")
	})
	_ = c.On("OnGroupMsgs", func(h *gosocketio.Channel, args returnPack) {
		if args.CurrentQQ != b.QQ {
			return
		}
		b.locker.RLock()
		defer b.locker.RUnlock()
		f, ok := b.onEvent["OnGroupMsgs"]
		if ok {
			f.Call([]reflect.Value{reflect.ValueOf(args.CurrentQQ), reflect.ValueOf(args.CurrentPacket)})
		}
		//log.Println(args)
	})
	_ = c.On("OnFriendMsgs", func(h *gosocketio.Channel, args returnPack) {
		if args.CurrentQQ != b.QQ {
			return
		}
		b.locker.RLock()
		defer b.locker.RUnlock()
		f, ok := b.onEvent["OnFriendMsgs"]
		if ok {
			f.Call([]reflect.Value{reflect.ValueOf(args.CurrentQQ), reflect.ValueOf(args.CurrentPacket)})
		}
		//log.Println(args)
	})
	_ = c.On("OnEvents", func(h *gosocketio.Channel, args interface{}) {
		tmp, _ := json.Marshal(args)
		log.Println(string(tmp))
	})

	return nil
}

// 发送消息函数
func (b *BotManager) Send(sendMsgPack SendMsgPack) {
	select {
	case b.SendChan <- sendMsgPack:
	default:
	}
}

// 停止
func (b *BotManager) Stop() {
	if !b.Running {
		return
	}
	b.Running = false
	close(b.SendChan)
}

// 撤回消息
func (b *BotManager) ReCallMsg(GroupID, MsgRandom int64, MsgSeq int) {
	res, err := requests.PostJson(b.OPQUrl+"/v1/LuaApiCaller?funcname=PbMessageSvc.PbMsgWithDraw&qq="+strconv.FormatInt(b.QQ, 10), map[string]interface{}{"GroupID": GroupID, "MsgSeq": MsgSeq, "MsgRandom": MsgRandom})
	if err != nil {
		log.Println(err.Error())
		return
	}
	log.Println(res.Text())
}
func (b *BotManager) AddEvent(EventName string, f interface{}) error {
	fVal := reflect.ValueOf(f)
	if fVal.Kind() != reflect.Func {
		return errors.New("NotFuncError")
	}
	var okStruck string
	switch EventName {
	case EventNameOnFriendMessage:
		okStruck = "OPQBot.CurrentPacket"
	case EventNameOnGroupMessage:
		okStruck = "OPQBot.CurrentPacket"
	default:
		okStruck = ""
	}
	log.Println(fVal.Type().In(1).String())
	if fVal.Type().NumIn() != 2 || fVal.Type().In(1).String() != okStruck {
		return errors.New("FuncError")
	}

	b.locker.Lock()
	defer b.locker.Unlock()
	b.onEvent[EventName] = fVal
	return nil

}

func (b *BotManager) receiveSendPack() {
	log.Println("QQ发送信息通道开启")
OuterLoop:
	for {
		if !b.Running {
			break
		}
		sendMsgPack := <-b.SendChan
		sendJsonPack := make(map[string]interface{})
		sendJsonPack["ToUserUid"] = sendMsgPack.ToUserUid
		switch sendMsgPack.SendType {
		case SendTypeTextMsg:
			sendJsonPack["SendMsgType"] = "TextMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch content := sendMsgPack.Content.(type) {
			case SendTypeTextMsgContent:
				sendJsonPack["Content"] = content.Content
			case SendTypeTextMsgContentPrivateChat:
				sendJsonPack["Content"] = content.Content
				sendJsonPack["GroupID"] = content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypePicMsgByUrl:
			sendJsonPack["SendMsgType"] = "PicMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypePicMsgByUrlContent:
				sendJsonPack["PicUrl"] = Content.PicUrl
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["FlashPic"] = Content.Flash
			case SendTypePicMsgByUrlContentPrivateChat:
				sendJsonPack["PicUrl"] = Content.PicUrl
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["GroupID"] = Content.Group
				sendJsonPack["FlashPic"] = Content.Flash
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypePicMsgByLocal:
			sendJsonPack["SendMsgType"] = "PicMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypePicMsgByLocalContent:
				sendJsonPack["PicPath"] = Content.Path
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["FlashPic"] = Content.Flash
			case SendTypePicMsgByLocalContentPrivateChat:
				sendJsonPack["PicPath"] = Content.Path
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["GroupID"] = Content.Group
				sendJsonPack["FlashPic"] = Content.Flash
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypePicMsgByMd5:
			sendJsonPack["SendMsgType"] = "PicMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypePicMsgByMd5Content:
				sendJsonPack["PicMd5s"] = Content.Md5
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["FlashPic"] = Content.Flash
			case SendTypePicMsgByMd5ContentPrivateChat:
				sendJsonPack["PicMd5s"] = Content.Md5s
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["GroupID"] = Content.Group
				sendJsonPack["FlashPic"] = Content.Flash
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeVoiceByUrl:
			sendJsonPack["SendMsgType"] = "VoiceMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeVoiceByUrlContent:
				sendJsonPack["VoiceUrl"] = Content.VoiceUrl
			case SendTypeVoiceByUrlContentPrivateChat:
				sendJsonPack["VoiceUrl"] = Content.VoiceUrl
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeVoiceByLocal:
			sendJsonPack["SendMsgType"] = "VoiceMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeVoiceByLocalContent:
				sendJsonPack["VoiceUrl"] = Content.Path
			case SendTypeVoiceByLocalContentPrivateChat:
				sendJsonPack["VoiceUrl"] = Content.Path
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeXml:
			sendJsonPack["SendMsgType"] = "XmlMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeXmlContent:
				sendJsonPack["Content"] = Content.Content
			case SendTypeXmlContentPrivateChat:
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeJson:
			sendJsonPack["SendMsgType"] = "XmlMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeJsonContent:
				sendJsonPack["Content"] = Content.Content
			case SendTypeJsonContentPrivateChat:
				sendJsonPack["Content"] = Content.Content
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeForword:
			sendJsonPack["SendMsgType"] = "ForwordMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeForwordContent:
				sendJsonPack["ForwordBuf"] = Content.ForwordBuf
				sendJsonPack["ForwordField"] = Content.ForwordField
			case SendTypeForwordContentPrivateChat:
				sendJsonPack["ForwordBuf"] = Content.ForwordBuf
				sendJsonPack["ForwordField"] = Content.ForwordField
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		case SendTypeReplay:
			sendJsonPack["SendMsgType"] = "ReplayMsg"
			sendJsonPack["SendToType"] = sendMsgPack.SendToType
			switch Content := sendMsgPack.Content.(type) {
			case SendTypeRelayContent:
				sendJsonPack["ReplayInfo"] = Content.ReplayInfo
			case SendTypeRelayContentPrivateChat:
				sendJsonPack["ReplayInfo"] = Content.ReplayInfo
				sendJsonPack["GroupID"] = Content.Group
			default:
				log.Println("类型不匹配")
				continue OuterLoop
			}
		}
		res, err := requests.PostJson(b.OPQUrl+"/v1/LuaApiCaller?funcname=SendMsgV2&qq="+strconv.FormatInt(b.QQ, 10), sendJsonPack)
		if err != nil {
			log.Println(err.Error())
			continue
		}
		log.Println(res.Text())
		time.Sleep(1 * time.Second)
	}
}
