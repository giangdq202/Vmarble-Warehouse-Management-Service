package events

import (
	"testing"
	"time"
)

func TestBroker_SubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe("user-1", "cnc")
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
	ch1, unsub1 := b.Subscribe("user-1", "cnc")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("user-2", "cnc")
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
	_, unsub := b.Subscribe("user-1", "cnc")
	unsub() // unsubscribe immediately

	// Publish must not panic after unsubscribe
	b.Publish(Event{UserID: "user-1", Type: EventTypeNewAssignment})
}

func TestBroker_MultipleSubscribersForSameUser(t *testing.T) {
	b := NewBroker()
	ch1, unsub1 := b.Subscribe("user-1", "cnc")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("user-1", "cnc")
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

// ── Role broadcast (issue #258) ──────────────────────────────────────────────

func TestBroker_RoleBroadcast_DeliversToMatchingRole(t *testing.T) {
	b := NewBroker()
	planner, unp := b.Subscribe("u-planner", "planner")
	defer unp()
	cnc, uncnc := b.Subscribe("u-cnc", "cnc")
	defer uncnc()
	accountant, una := b.Subscribe("u-accountant", "accountant")
	defer una()

	// WO_STATUS_CHANGED targets planner + accountant (no UserID).
	b.Publish(Event{
		Roles: []string{"planner", "accountant"},
		Type:  EventTypeWOStatusChanged,
		WoID:  "wo-7",
	})

	for name, ch := range map[string]<-chan Event{"planner": planner, "accountant": accountant} {
		select {
		case e := <-ch:
			if e.Type != EventTypeWOStatusChanged || e.WoID != "wo-7" {
				t.Errorf("%s got %+v, want WO_STATUS_CHANGED wo-7", name, e)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s timed out", name)
		}
	}

	select {
	case e := <-cnc:
		t.Errorf("cnc subscriber must not receive role-targeted event: %+v", e)
	default:
	}
}

func TestBroker_PersonalAndRoleBoth_DeduplicatesPerSubscription(t *testing.T) {
	// Assignee subscribes as a CNC operator. Event sets BOTH UserID (=assignee)
	// and Roles: [cnc] — broker must deliver exactly ONE event to that
	// subscription, not two.
	b := NewBroker()
	ch, unsub := b.Subscribe("u-1", "cnc")
	defer unsub()

	b.Publish(Event{
		UserID: "u-1",
		Roles:  []string{"cnc"},
		Type:   EventTypeNewAssignment,
		WoID:   "wo-1",
	})

	select {
	case e := <-ch:
		if e.WoID != "wo-1" {
			t.Errorf("got WoID %q, want wo-1", e.WoID)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber must receive at least one event")
	}

	select {
	case e := <-ch:
		t.Errorf("subscriber received duplicate event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected — only one delivery
	}
}

func TestBroker_RoleBroadcast_SkipsSubscribersWithoutRole(t *testing.T) {
	b := NewBroker()
	noRole, un := b.Subscribe("u-1", "")
	defer un()

	b.Publish(Event{Roles: []string{"planner"}, Type: EventTypeWOStatusChanged})

	select {
	case e := <-noRole:
		t.Errorf("subscriber with empty role must not receive role-only event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}
