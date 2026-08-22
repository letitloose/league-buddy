package services

import "testing"

func TestTextFromHTML(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "activation-style link",
			html: `<html>
					<body>
						<h1>Hello!</h1>
						<p>Please <a href="https://blametheball.com/user/activate?hash=abc">click here</a> to validate your email and activate your account.<p>
					</body>
				</html>`,
			want: "Hello! Please click here (https://blametheball.com/user/activate?hash=abc) to validate your email and activate your account.",
		},
		{
			name: "invite-style sentence with a link",
			html: `<html><body><p>You've been invited to join Colonial FC on Blame the Ball. <a href="https://blametheball.com/user/signup?invite=tok">Sign up here</a>.</p></body></html>`,
			want: "You've been invited to join Colonial FC on Blame the Ball. Sign up here (https://blametheball.com/user/signup?invite=tok).",
		},
		{
			name: "no link at all",
			html: `<p>Just some text.</p>`,
			want: "Just some text.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textFromHTML(tt.html)
			if got != tt.want {
				t.Errorf("textFromHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}
