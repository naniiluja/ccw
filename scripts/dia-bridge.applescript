-- Open the bridge in Dia's agent chat.
--
-- The order matters and was found by watching the app rather than by reading it.
-- Dia keeps the last turn's text in the field, so typing "bridge" appends to
-- whatever is already there ("bridgebridge") and the word never matches; the
-- field is cleared first for that reason.
--
-- Space is what turns the word into the bridge chip. Without it Dia treats the
-- turn as a browser omnibox query and opens a Google search instead, which is
-- where a plain "bridge" used to end up.
--
-- Once the chip is showing, the field reads "Enter to Send Now" and pressing the
-- down arrow moves focus off it, so the return goes straight after the space.

on run
	tell application "Dia"
		activate
		delay 1
		tell application "System Events"
			delay 0.6
			-- A fresh tab leaves the field focused already; clicking it costs
			-- nothing and is what makes the word land when it does not.
			click at {880, 415}
			delay 0.6
			keystroke "a" using {command down}
			delay 0.3
			key code 51
			delay 0.5
			keystroke "bridge"
			delay 1.2
			key code 49
			delay 1.2
			key code 36
		end tell
	end tell
end run
