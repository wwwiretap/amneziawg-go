package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync/atomic"
	"time"
)

/* Sends a keep-alive if no packets queued for peer
 *
 * Used by initiator of handshake and with active keep-alive
 */
func (peer *Peer) SendKeepAlive() bool {
	if len(peer.queue.nonce) == 0 {
		select {
		case peer.queue.nonce <- []byte{}:
			return true
		default:
			return false
		}
	}
	return true
}

func StoppedTimer() *time.Timer {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	return timer
}

/* Called when a new authenticated message has been send
 *
 * TODO: This might be done in a faster way
 */
func (peer *Peer) KeepKeyFreshSending() {
	send := func() bool {
		peer.keyPairs.mutex.RLock()
		defer peer.keyPairs.mutex.RUnlock()

		kp := peer.keyPairs.current
		if kp == nil {
			return false
		}

		if !kp.isInitiator {
			return false
		}

		nonce := atomic.LoadUint64(&kp.sendNonce)
		if nonce > RekeyAfterMessages {
			return true
		}
		return time.Now().Sub(kp.created) > RekeyAfterTime
	}()
	if send {
		sendSignal(peer.signal.handshakeBegin)
	}
}

/* This is the state machine for handshake initiation
 *
 * Associated with this routine is the signal "handshakeBegin"
 * The routine will read from the "handshakeBegin" channel
 * at most every RekeyTimeout seconds
 */
func (peer *Peer) RoutineHandshakeInitiator() {
	device := peer.device
	buffer := make([]byte, 1024)
	logger := device.log.Debug
	timeout := time.NewTimer(time.Hour)

	var work *QueueOutboundElement

	logger.Println("Routine, handshake initator, started for peer", peer.id)

	func() {
		for {
			var attempts uint
			var deadline time.Time

			// wait for signal

			select {
			case <-peer.signal.handshakeBegin:
			case <-peer.signal.stop:
				return
			}

		HandshakeLoop:
			for {
				// clear completed signal

				select {
				case <-peer.signal.handshakeCompleted:
				case <-peer.signal.stop:
					return
				default:
				}

				// create initiation

				if work != nil {
					work.mutex.Lock()
					work.packet = nil
					work.mutex.Unlock()
				}
				work = new(QueueOutboundElement)
				msg, err := device.CreateMessageInitiation(peer)
				if err != nil {
					device.log.Error.Println("Failed to create initiation message:", err)
					break
				}

				// schedule for sending

				writer := bytes.NewBuffer(buffer[:0])
				binary.Write(writer, binary.LittleEndian, msg)
				work.packet = writer.Bytes()
				peer.mac.AddMacs(work.packet)
				peer.InsertOutbound(work)

				if attempts == 0 {
					deadline = time.Now().Add(MaxHandshakeAttemptTime)
				}

				// set timeout

				if !timeout.Stop() {
					select {
					case <-timeout.C:
					default:
					}
				}
				attempts += 1
				timeout.Reset(RekeyTimeout)
				device.log.Debug.Println("Handshake initiation attempt", attempts, "queued for peer", peer.id)

				// wait for handshake or timeout

				select {
				case <-peer.signal.stop:
					return

				case <-peer.signal.handshakeCompleted:
					break HandshakeLoop

				case <-timeout.C:
					if deadline.Before(time.Now().Add(RekeyTimeout)) {
						peer.signal.flushNonceQueue <- struct{}{}
						if !peer.timer.sendKeepalive.Stop() {
							<-peer.timer.sendKeepalive.C
						}
						break HandshakeLoop
					}
				}
			}
		}
	}()

	logger.Println("Routine, handshake initator, stopped for peer", peer.id)
}

/* Handles incomming packets related to handshake
 *
 *
 */
func (device *Device) HandshakeWorker(queue chan struct {
	msg     []byte
	msgType uint32
	addr    *net.UDPAddr
}) {
	for {
		elem := <-queue

		switch elem.msgType {
		case MessageInitiationType:
			if len(elem.msg) != MessageInitiationSize {
				continue
			}

			// check for cookie

			var msg MessageInitiation

			binary.Read(nil, binary.LittleEndian, &msg)

		case MessageResponseType:
			if len(elem.msg) != MessageResponseSize {
				continue
			}

			// check for cookie

		case MessageCookieReplyType:
			if len(elem.msg) != MessageCookieReplySize {
				continue
			}

		default:
			device.log.Error.Println("Invalid message type in handshake queue")
		}
	}
}
