(
s.waitForBoot({
  play {
    var l = LFNoise2;  
    FreeVerb2.ar(            
      *XFade2.ar(
        SinOscFB.ar(
          [80, 81],  
          l.kr(1) + 1/2  
        ),
        SinOscFB.ar(
          [121, 160],  
          l.kr(1) + 1/2  
        ),
        l.kr(1)  
      )
    )
  };
});
)
