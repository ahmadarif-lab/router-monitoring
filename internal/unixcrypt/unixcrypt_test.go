package unixcrypt

import "testing"

// Vectors produced by glibc's crypt() (Python 3.12 on Linux), the reference implementation.
var cryptVectors = []struct{ pw, setting, want string }{
	{"", "$1$saltsalt$", "$1$saltsalt$5Jhcit4zN9UlGiA0txPkO0"},
	{"a", "$1$saltsalt$", "$1$saltsalt$MzDjbImyzEz2m0NTiPzNW/"},
	{"Hello world!", "$1$saltsalt$", "$1$saltsalt$le8lFSqqnPaRFOlmAZpvH1"},
	{"p@ssw0rd long password with spaces 1234567890", "$1$saltsalt$", "$1$saltsalt$CkNxe0gq9/zMV0qQQ6EHq/"},
	{"ünïcode-pässword", "$1$saltsalt$", "$1$saltsalt$0FbQhY5q6eW.mNHT6jgU50"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$1$saltsalt$", "$1$saltsalt$lTT80f5GEB.8cfLv43D0D1"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$1$saltsalt$", "$1$saltsalt$Q9vZJhpqSQMV3xAby.p631"},
	{"", "$1$abc$", "$1$abc$Or2rbeUYTvt12aiVzMuS/."},
	{"a", "$1$abc$", "$1$abc$uAXS.Zj1ZDNcW9iX1TVH2/"},
	{"Hello world!", "$1$abc$", "$1$abc$.FiamM1s4pFo0QKAk4pqS."},
	{"p@ssw0rd long password with spaces 1234567890", "$1$abc$", "$1$abc$KkXdxyiZQIk6jx4P8/b5H."},
	{"ünïcode-pässword", "$1$abc$", "$1$abc$kVWf.ZqbLivEWqO/GoZVu1"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$1$abc$", "$1$abc$b/oSMeDNO7QqHD3W/TXaY0"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$1$abc$", "$1$abc$aROpSwqxeZ4CehIOQUpMi/"},
	{"", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$LhbeyAi/Q1f1Pl2t0M273sdH.FR6/lsi8AqDILGHkU."},
	{"a", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$eG.nWUVATpZ9dLBOpsYV5xwgU8ZdTQwpyhVP10klu90"},
	{"Hello world!", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$SJz19vqixVPbVurSqACMvwmL84YklhvdiauTO62ZAE."},
	{"p@ssw0rd long password with spaces 1234567890", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$2hmNglW.6M4bO9t7VrNFWZSB27pRIYU.VHPMJlZpnK."},
	{"ünïcode-pässword", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$LI0vpQ9kaC3GMOfZUnRd5cINK1uwPJnS8gVjcqXQ/A3"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$6vVpuInhCKaUkvfNELzKkXibaz6KjtE7I/7cDt6svFA"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$5$saltsaltsalt1234$", "$5$saltsaltsalt1234$xtPfXjwIXeMeJzL48eD2iXoQoy19dCqUHaEUc9LlAK/"},
	{"", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$TJGOJSuY1X0IaQtlKb8AWe8cITzVmSjAabXgm5jRfR9"},
	{"a", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$MSS/kVy.ztC/3kD54y583Y6OTF8.FnMTw4qGy.2fl08"},
	{"Hello world!", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$.abCD9vmI4DeARrdqcOB3SMIYkKSuZE3BZj3dzC.oT."},
	{"p@ssw0rd long password with spaces 1234567890", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$.3iaacbdyc2ZEPDvXBvB05rhhDgU6BGK8NSXSvWDM2A"},
	{"ünïcode-pässword", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$8gUhOKvX54CgNqfLN69uSQnNkhfXvcwSw7fmMRrboC0"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$HY8YiHljLimZ9qJlNc7dtDJ0xng6Y7jTNpGboXzCBa1"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$5$rounds=10000$abc$", "$5$rounds=10000$abc$LL/FbwzI0SMGGvdN4Y1Hnlt2wix2aepwNhXjionn1g/"},
	{"", "$5$s$", "$5$s$u9jS/E6rlpZoj8KCMVhpLKJh2MrO5hc9UBdfKCcvVU7"},
	{"a", "$5$s$", "$5$s$qgP3B1D0m82PjqmN9fAYcv/rBMS9t2tLp7NrzhQCCyC"},
	{"Hello world!", "$5$s$", "$5$s$lvy3OSvXN.tFKr9I/ZbDeDKQni6mSLrEJ/tISAjUxxB"},
	{"p@ssw0rd long password with spaces 1234567890", "$5$s$", "$5$s$rBskvotR6kj9xZTuIlmPSWO0acKw5WpLqzuDAFwjLoD"},
	{"ünïcode-pässword", "$5$s$", "$5$s$HYz5t/by6H6ip0rpShiMZlmfY/AsS.q5nb.hfvCr2h8"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$5$s$", "$5$s$Wbm4N3S9ArYHDVrR3v4S2fN2c6tEDQbCSMZbwjsTL9D"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$5$s$", "$5$s$E3um.HBGFy9fuuaONOAc0I4c7D2Xj6/3PB7dPe6NQd1"},
	{"", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$oMkYKCcZV8CfUBk1FvT.PvQ7icyvBsT3ul9rcwgQFz1yHWwuxn9u7j/uGj8VAEPZEYPQk7RQfpNr1jlc52kNL1"},
	{"a", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$/QfZvC9Ok6x1P.pPmL.EqdydELhorur31pJJ5RGvQSMAYNW24JwY8.KcnhG4aCqLjEqay1bOoJweKfy3ZWWXw."},
	{"Hello world!", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$GoCVRi9c2TaDnzyNRMhKrdbeC3a4ITe5FkdNukLlr2OlSfKapJZhI9ngMKozCpq3LrUmrgGCIoOh4ENMl1Afm0"},
	{"p@ssw0rd long password with spaces 1234567890", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$SF3l9s5/pB7WyhiJ8eONfn/HS/fa8RlvAZg2Hv7oRniS5bP8aW4xyS8kwSWWDufZNAkKeCnw9tm8JIrWK4GAM1"},
	{"ünïcode-pässword", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$xa5gE1nJVyTNVDkXBmjuraAIvDBeyG6FBI7CZSK/tnIz0aJj7AMfEY/rXKSAEJY2xIVY4p41yzQB41UBwx45i0"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$yh8K8ugha2xmGOSp2on7ZzilgPQdriBqeYCwWXYAHaKufMXI.j0lueGzGU6du2Dde2uV4GRydFsajg5WU6yLY/"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$6$saltsaltsalt1234$", "$6$saltsaltsalt1234$O7FlIVFQQj34w5NP.BD06dZACY7Pbl2IHnH2WYK6QQ75YvJ7x5BG5e8yu2RgQKZsbeoxvmYV9uTobAl3mN0ZA0"},
	{"", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$PJKokJSWxHd/d5keaAejywyqkpe0DFF4idEVujA3bTaXsEWcCIYHjhM31XlACWIwJvUiIOtLvMYdOX7RDY2Bi1"},
	{"a", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$tDUJda1ba0d0uIvMWPRO0z0T2uCmyahZwkkN/Xoum1UEazcj4gYoOuL.hndwHGL43R23aHdIR3Knc66QPXAlC/"},
	{"Hello world!", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$vwjKuN1g4RJaa3WZscmb/1FtE064dYqQH6MDZQOMmtVKrcw77aTU3A0VWvPxL5MjgdPtFaXhfAaX/JVgcEii1/"},
	{"p@ssw0rd long password with spaces 1234567890", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$h5sniuoHz.Ne9IIJBl5Wa8Ng9uL08mpjHGHPjhp4roQyeZz7jk/8U6OzhOckD5rhzTbmhZ0Yk8UcyswjYtMe51"},
	{"ünïcode-pässword", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$vcwLag0PdEMQaiR4HTaDj565RESbXG252Eqj5Cn8RAcL3FJ1pIcvnf8E6.jQnxfRegbTshpXUzjjZoKx8ZxG31"},
	{"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$hKsVED79IoCMLg.K17knAClCk7YBq3rfhYCWkrog7ijpX2rvLyZR6gv9WHrdO49o6cehYaTQ7viyEggc3ulH11"},
	{"0123456789abcdef0123456789abcdef0123456789abcdef", "$6$rounds=7000$xyz$", "$6$rounds=7000$xyz$uYeGFtsUqRezVWaf0GDRLDVBkELd38HPwvkwN0a5dMmuCF1XzXGfMuX6FMaTMTXgiy0TRWKOKO4GhwhYkBl9t/"},
}

func TestCryptMatchesGlibc(t *testing.T) {
	for _, v := range cryptVectors {
		got, err := Crypt(v.pw, v.setting)
		if err != nil || got != v.want {
			t.Errorf("Crypt(%q, %q) = %q, %v; want %q", v.pw, v.setting, got, err, v.want)
		}
	}
	if _, err := Crypt("x", "$9$salt$"); err == nil {
		t.Error("unknown algorithm must fail")
	}
}
