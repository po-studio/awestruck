"SC_JACK_DEFAULT_OUTPUTS".setenv(
    "webrtc-server:in_jackaudiosrc0_1",
    "webrtc-server:in_jackaudiosrc0_2"
);

postln("Outputs: " ++ "SC_JACK_DEFAULT_OUTPUTS".getenv);
postln("Inputs: " ++ "SC_JACK_DEFAULT_INPUTS".getenv);

ServerQuit.add({
    'FAIL: scsynth quit'.postln;
    1.exit();
}, Server.default);