"SC_JACK_DEFAULT_OUTPUTS".setenv(
     "awestruck:in_jackaudiosrc0_1",
     "awestruck:in_jackaudiosrc0_2"
    );

ServerQuit.add({
    'FAIL: scsynth quit'.postln;
    1.exit();
}, Server.default);