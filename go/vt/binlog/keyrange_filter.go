// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binlog

import (
	"encoding/base64"
	"strconv"
	"strings"

	log "github.com/golang/glog"
	"github.com/youtube/vitess/go/vt/binlog/proto"
	"github.com/youtube/vitess/go/vt/key"

	pb "github.com/youtube/vitess/go/vt/proto/topodata"
)

var KEYSPACE_ID_COMMENT = "/* EMD keyspace_id:"
var SPACE = " "

// KeyRangeFilterFunc returns a function that calls sendReply only if statements
// in the transaction match the specified keyrange. The resulting function can be
// passed into the BinlogStreamer: bls.Stream(file, pos, sendTransaction) ->
// bls.Stream(file, pos, KeyRangeFilterFunc(sendTransaction))
func KeyRangeFilterFunc(kit key.KeyspaceIdType, keyrange *pb.KeyRange, sendReply sendTransactionFunc) sendTransactionFunc {
	isInteger := true
	if kit == key.KIT_BYTES {
		isInteger = false
	}

	return func(reply *proto.BinlogTransaction) error {
		matched := false
		filtered := make([]proto.Statement, 0, len(reply.Statements))
		for _, statement := range reply.Statements {
			switch statement.Category {
			case proto.BL_SET:
				filtered = append(filtered, statement)
			case proto.BL_DDL:
				log.Warningf("Not forwarding DDL: %s", statement.Sql)
				continue
			case proto.BL_DML:
				keyspaceIndex := strings.LastIndex(statement.Sql, KEYSPACE_ID_COMMENT)
				if keyspaceIndex == -1 {
					updateStreamErrors.Add("KeyRangeStream", 1)
					log.Errorf("Error parsing keyspace id: %s", statement.Sql)
					continue
				}
				idstart := keyspaceIndex + len(KEYSPACE_ID_COMMENT)
				idend := strings.Index(statement.Sql[idstart:], SPACE)
				if idend == -1 {
					updateStreamErrors.Add("KeyRangeStream", 1)
					log.Errorf("Error parsing keyspace id: %s", statement.Sql)
					continue
				}
				textId := statement.Sql[idstart : idstart+idend]
				if isInteger {
					id, err := strconv.ParseUint(textId, 10, 64)
					if err != nil {
						updateStreamErrors.Add("KeyRangeStream", 1)
						log.Errorf("Error parsing keyspace id: %s", statement.Sql)
						continue
					}
					if !key.KeyRangeContains(keyrange, key.Uint64Key(id).Bytes()) {
						continue
					}
				} else {
					data, err := base64.StdEncoding.DecodeString(textId)
					if err != nil {
						updateStreamErrors.Add("KeyRangeStream", 1)
						log.Errorf("Error parsing keyspace id: %s", statement.Sql)
						continue
					}
					if !key.KeyRangeContains(keyrange, data) {
						continue
					}
				}
				filtered = append(filtered, statement)
				matched = true
			case proto.BL_UNRECOGNIZED:
				updateStreamErrors.Add("KeyRangeStream", 1)
				log.Errorf("Error parsing keyspace id: %s", statement.Sql)
				continue
			}
		}
		if matched {
			reply.Statements = filtered
		} else {
			reply.Statements = nil
		}
		return sendReply(reply)
	}
}
