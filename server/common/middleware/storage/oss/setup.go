package oss

import "log"

func Setup(entity *OssEntity) {
	Oss = nil
	if entity == nil {
		log.Println("oss entity is nil, skip oss setup")
		return
	}
	if !entity.Enabled {
		log.Println("oss disabled, skip oss setup")
		return
	}
	entity.Default()

	var err error

	var oss *AliyunOss
	if oss, err = NewAliyun(entity); err != nil {
		log.Printf("oss setup failed, skip oss capability: %v", err)
		return
	}
	Oss = oss
}
