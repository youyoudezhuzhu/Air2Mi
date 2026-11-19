package org.air2mi.app;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.regex.*;
final class DeviceDisplay {
 static String filter(String text){
  Matcher matcher=Pattern.compile("(?i)AMD1-[A-Z0-9-]+").matcher(text);StringBuffer result=new StringBuffer();
  while(matcher.find())matcher.appendReplacement(result,Matcher.quoteReplacement(code(matcher.group())));matcher.appendTail(result);
  return result.toString().replaceAll("(?i)A2M-(?:[A-F0-9]{5}-){7}[A-F0-9]{5}","[redacted]");
 }
 static String code(String input){try{
  String hex=input.substring(5).replace("-","").toUpperCase(java.util.Locale.ROOT);
  if(!hex.matches("[A-F0-9]{32}"))return "[device-code-unavailable]";
  byte[] mask=MessageDigest.getInstance("SHA-256").digest("air2mi.device.display.v1".getBytes(StandardCharsets.UTF_8));StringBuilder output=new StringBuilder("AVD1-");
  for(int i=0;i<16;i++){if(i>0&&i%2==0)output.append('-');output.append(String.format(java.util.Locale.ROOT,"%02X",Integer.parseInt(hex.substring(i*2,i*2+2),16)^(mask[i]&255)));}
  return output.toString();
 }catch(Exception error){return "[device-code-unavailable]";}}
}
