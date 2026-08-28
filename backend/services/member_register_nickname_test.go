package services

import "testing"

func TestRandomMemberNicknameUsesNeutralLibrary(t *testing.T) {
	adjectives := make(map[string]struct{}, len(memberNicknameAdjectives))
	objects := make(map[string]struct{}, len(memberNicknameObjects))
	for _, item := range memberNicknameAdjectives {
		adjectives[item] = struct{}{}
	}
	for _, item := range memberNicknameObjects {
		objects[item] = struct{}{}
	}

	for index := 0; index < 50; index++ {
		name := randomMemberNickname()
		matched := false
		for adjective := range adjectives {
			for object := range objects {
				if name == adjective+object {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			t.Fatalf("generated nickname %q is not from the neutral nickname library", name)
		}
	}
}
