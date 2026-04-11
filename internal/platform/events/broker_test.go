package events

import (
	"testing"
	"time"
)

func TestBroker_SubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("user-1")
	defer unsub()

	b.Publish(Event{UserID: "user-1", Type: EventTypeNewAssignment, WoID: "wo-1", SKU: "SKU-001"})

	select {
	case e := <-ch:
		if e.WoID != "wo-1" {
			t.Errorf("WoID = %q, want %q", e.WoID, "wo-1")
		}
		if e.Type != EventTypeNewAssignment {
			t.Errorf("Type = %q, want %q", e.Type, EventTypeNewAssignment)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_Publish_OnlyTargetedUser(t *testing.T) {
	b := NewBroker()
	ch1, unsub1 := b.Subscribe("user-1")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("user-2")
	defer unsub2()

	b.Publish(Event{UserID: "user-1", Type: EventTypeNewAssignment, WoID: "wo-A"})

	select {
	case e := <-ch1:
		if e.WoID != "wo-A" {
			t.Errorf("ch1 WoID = %q, want %q", e.WoID, "wo-A")
		}
	case <-time.After(time.Second):
		t.Fatal("ch1 timed out")
	}

	// ch2 must NOT receive anything
	select {
	case e := <-ch2:
		t.Errorf("ch2 unexpectedly received event: %+v", e)
	default:
	}
}

func TestBroker_Unsubscribe_StopsDelivery(t *testing.T) {
	b := NewBroker()
	_, unsub := b.Subscribe("user-1")
	unsub() // unsubscribe immediately

	// Publish must not panic after unsubscribe
	b.Publish(Event{UserID: "user-1", Type: EventTypeNewAssignment})
}

func TestBroker_MultipleSubscribersForSameUser(t *testing.T) {
	b := NewBroker()
	ch1, unsub1 := b.Subscribe("user-1")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("user-1")
	defer unsub2()

	b.Publish(Event{UserID: "user-1", WoID: "wo-X"})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.WoID != "wo-X" {
				t.Errorf("WoID = %q, want %q", e.WoID, "wo-X")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event on subscriber channel")
		}
	}
}
