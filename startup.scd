"SUPERCOLLIDER_READY_FOR_JACK_CONNECTIONS".postln;

OSCFunc({ |msg, time, addr, recvPort|
    // msg[1] and msg[2] should contain the new output port names
    var out1 = msg[1].asString;
    var out2 = msg[2].asString;
    "SC_JACK_DEFAULT_OUTPUTS".setenv(out1 ++ "," ++ out2);

    postln("Outputs: " ++ "SC_JACK_DEFAULT_OUTPUTS".getenv);
    postln("Inputs: " ++ "SC_JACK_DEFAULT_INPUTS".getenv);

    Server.default.options.outDevice_("JACK");
    Server.default.reboot;
}, '/updateJACKOutputs').permanent_(true);

ServerQuit.add({
    thisProcess.shutdown();
    0.exit();
}, Server.default);