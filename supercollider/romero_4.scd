(
s.waitForBoot({
  Buffer.allocConsecutive(8, s, 1024, 1, {|b, i|
    b.sine1Msg(1 / (1..((i + 1) * 6)))  
  }, 0);
  
  {
    var f = [50, 75, 99];  
    VOsc3.ar(
      LFNoise1.kr(1 / 4, 3, 4),  
      *f  
    ).lag(3e-3) ! 2  
  }.play;
});
)
