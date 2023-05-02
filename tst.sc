(
Server.default.waitForBoot({
    "SuperCollider playing...".postln;
    
    (
        f = { |msg, time, addr|
            if(msg[0] != '/status.reply') {
                "time: % sender: %\nmessage: %\n".postf(time, addr, msg);
            }
        };
        thisProcess.addOSCRecvFunc(f);
    );
    
    (x=play{
        
        |
        var1=1.0,
        var2=1.0,
        var3=1.0
        |
        
        SinOsc.ar([400,500], 0.5, 0.5);
        
    });
    
    (
        OSCdef(
            key: \sinw,
            func: { |msg|
                x.set(\var1,msg[1]);
                x.set(\var2,msg[2]);
                x.set(\var3,msg[3]);
            },
            path: '/sinw');
    );
})
);