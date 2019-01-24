package connection

import (
	"bytes"
	"testing"

	"github.com/thetatoken/theta/rlp"

	"github.com/stretchr/testify/assert"
	"github.com/thetatoken/theta/common"
)

func TestDefaultChannelEnqueueShortMsg(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	assert.Equal(common.ChannelIDTransaction, ch.getID())

	msgBytes := []byte("hello world")
	success := ch.enqueueMessage(msgBytes)
	assert.True(success)
	assert.True(ch.hasPacketToSend())

	strBuf := bytes.NewBufferString("")
	nonempty, numBytes, err := ch.sendPacketTo(strBuf)
	assert.True(nonempty)
	assert.True(numBytes > len(msgBytes))
	assert.Nil(err)
	t.Logf("numBytes: %v", numBytes)

	var decodedPacket Packet
	rlp.Decode(strBuf, &decodedPacket)
	t.Logf("decodedPacket.ChannelID: %v", decodedPacket.ChannelID)
	t.Logf("decodedPacket.Bytes: %v", string(decodedPacket.Bytes))
	t.Logf("decodedPacket.IsEOF: %v", decodedPacket.IsEOF)

	assert.Equal(common.ChannelIDTransaction, decodedPacket.ChannelID)
	assert.Equal(msgBytes, decodedPacket.Bytes)
	assert.Equal(byte(0x01), decodedPacket.IsEOF)
}

func TestDefaultChannelEnqueueLongMsg(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	assert.Equal(common.ChannelIDTransaction, ch.getID())

	partStr := "0123456789012345012345678901234501234567890123450123456789012345012345678901234501234567890123450123456789012345012345678901234501234567890123450123456789012345012345678901234501234567890123450123456789012345012345678901234501234567890123450123456789012345" // 256 Bytes
	longStr := ""
	numParts := 4096
	for i := 0; i < numParts; i++ {
		longStr += partStr
	}
	assert.Equal(1024*1024, len(longStr)) // 1MB message

	longBytes := []byte(longStr)
	success := ch.enqueueMessage(longBytes)
	assert.True(success)
	assert.True(ch.hasPacketToSend())

	strBuf := bytes.NewBufferString("")
	totalBytes := 0
	recvStr := ""
	for {
		_, numBytes, err := ch.sendPacketTo(strBuf)
		totalBytes += numBytes
		assert.Nil(err)

		var decodedPacket Packet
		rlp.Decode(strBuf, &decodedPacket)
		assert.Equal(common.ChannelIDTransaction, decodedPacket.ChannelID)

		recvStr += string(decodedPacket.Bytes)
		if decodedPacket.IsEOF == byte(0x01) {
			break
		}
	}

	assert.True(totalBytes > len(partStr)*numParts)
	assert.Equal(longStr, recvStr)

	t.Logf("totalBytes: %v", totalBytes)
	//t.Logf("Long string sent: %v", longStr)
	//t.Logf("received string: %v", recvStr)
}

func TestDefaultChannelAttemptEnqueueMsg(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	msgBytes := []byte("hello world")
	success := ch.enqueueMessage(msgBytes)
	assert.True(success)
	assert.True(ch.hasPacketToSend())

	assert.False(ch.canEnqueueMessage())
	attemptSuccess := ch.attemptToEnqueueMessage(msgBytes)
	assert.False(attemptSuccess)
}

func TestDefaultChannelRecvSingleMsg(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	msgBytes := []byte("0123456789")
	packet := &Packet{
		ChannelID: common.ChannelIDTransaction,
		Bytes:     msgBytes,
		IsEOF:     byte(0x01),
	}

	recvBytes, success := ch.receivePacket(packet)
	assert.True(success)
	assert.Equal(msgBytes, recvBytes)
}

func TestDefaultChannelRecvMultipleMsgs(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	partBytes := []byte("0123456789")
	partPacket := &Packet{
		ChannelID: common.ChannelIDTransaction,
		Bytes:     partBytes,
		IsEOF:     byte(0x00),
	}

	totalNumPackets := uint(11)
	i := uint(0)
	for ; i < totalNumPackets-1; i++ {
		partPacket.SeqID = i
		recvBytes, success := ch.receivePacket(partPacket)
		assert.True(success)
		assert.Nil(recvBytes)
	}

	endBytes := []byte("abcdef")
	endPacket := &Packet{
		ChannelID: common.ChannelIDTransaction,
		Bytes:     endBytes,
		IsEOF:     byte(0x01),
		SeqID:     i,
	}

	recvBytes, success := ch.receivePacket(endPacket)
	assert.True(success)

	completeMsg := ""
	for i := uint(0); i < totalNumPackets-1; i++ {
		completeMsg += string(partBytes)
	}
	completeMsg += string(endBytes)
	completeMsgBytes := []byte(completeMsg)
	t.Logf("complete message: %v", completeMsg)
	t.Logf("received message: %v", string(recvBytes))

	assert.Equal(completeMsgBytes, recvBytes)
}

func TestDefaultChannelRecvExtraLongMsg(t *testing.T) {
	assert := assert.New(t)
	ch := createDefaultChannel(common.ChannelIDTransaction)

	expectedMsgBytes := []byte{}
	msgBytes := []byte("01234567890123450123456789012345012345678901234501234567890123450123456789012345012345678901234501234567890123450123456789012345") // 128 Bytes
	packet := &Packet{
		ChannelID: common.ChannelIDTransaction,
		Bytes:     msgBytes,
		IsEOF:     byte(0x00),
	}

	var success bool
	var recvBytes []byte
	i := uint(0)
	for ; i < 32767; i++ {
		packet.SeqID = i
		recvBytes, success = ch.receivePacket(packet)
		assert.True(success)
		assert.Nil(recvBytes)

		expectedMsgBytes = append(expectedMsgBytes, packet.Bytes...)
	}

	endPacket := &Packet{
		ChannelID: common.ChannelIDTransaction,
		Bytes:     msgBytes,
		IsEOF:     byte(0x01),
		SeqID:     i,
	}
	aggregatedBytes, success := ch.receivePacket(endPacket)
	assert.True(success)
	assert.NotNil(aggregatedBytes)
	expectedMsgBytes = append(expectedMsgBytes, endPacket.Bytes...)

	t.Logf("Length of the expectedMsgBytes: %v", len(expectedMsgBytes))
	t.Logf("Length of the aggregatedBytes:  %v", len(aggregatedBytes))

	assert.Equal(4194304, len(expectedMsgBytes)) // should be 4 MB
	assert.Equal(4194304, len(aggregatedBytes))  // should be 4 MB
	sameBytes := (bytes.Compare(expectedMsgBytes, aggregatedBytes) == 0)
	assert.True(sameBytes)
}
