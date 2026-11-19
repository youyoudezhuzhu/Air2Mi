package org.air2mi.app;
import android.app.*;
import android.content.*;
import android.net.Uri;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.json.*;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.*;
import static org.junit.Assert.*;
@RunWith(AndroidJUnit4.class)
public class RuntimeSmokeTest {
 private void waitFor(java.util.function.BooleanSupplier condition,int seconds)throws Exception{long end=System.currentTimeMillis()+seconds*1000L;while(System.currentTimeMillis()<end){if(condition.getAsBoolean())return;Thread.sleep(200);}assertTrue("condition timed out",condition.getAsBoolean());}
 @Test public void invalidImportPreservesInstalledComponentsAndUserData()throws Exception{
  Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();EngineRuntime runtime=EngineRuntime.get(context);
  JSONObject before=runtime.installedManifest();File identity=new File(context.getFilesDir(),"engine-data/import-preservation-marker");identity.getParentFile().mkdirs();java.nio.file.Files.write(identity.toPath(),"preserve".getBytes(StandardCharsets.UTF_8));
  File invalid=new File(context.getCacheDir(),"invalid-engine.zip");
  try(java.util.zip.ZipOutputStream zip=new java.util.zip.ZipOutputStream(new FileOutputStream(invalid))){zip.putNextEntry(new java.util.zip.ZipEntry("assets/engine-build.json"));zip.write("{\"platform\":\"windows\",\"architecture\":\"x64\"}".getBytes(StandardCharsets.UTF_8));zip.closeEntry();}
  try{runtime.importComponents(Uri.fromFile(invalid));fail("invalid component accepted");}catch(Exception expected){assertTrue(expected.getMessage().contains("平台"));}
  assertEquals(before==null?null:before.toString(),runtime.installedManifest()==null?null:runtime.installedManifest().toString());assertEquals("preserve",new String(java.nio.file.Files.readAllBytes(identity.toPath()),StandardCharsets.UTF_8));
  java.nio.file.Files.delete(identity.toPath());java.nio.file.Files.delete(invalid.toPath());
 }
 @Test public void importedEngineAndBackgroundLifecycle()throws Exception{
  android.app.Instrumentation instrument=InstrumentationRegistry.getInstrumentation();Context context=instrument.getTargetContext();EngineRuntime runtime=EngineRuntime.get(context);
  JSONObject report=new JSONObject();boolean wasWanted=EngineService.wanted(context);Activity activity=null;
  try{
   context.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",false).commit();
   File component=new File(context.getFilesDir(),"test-engine-components.zip");assertTrue("push standalone Engine components before running this test",component.isFile());
   JSONObject manifest=runtime.importComponents(Uri.fromFile(component));assertEquals("android",manifest.getString("platform"));assertTrue(manifest.getJSONObject("hardening").getBoolean("native_entitlement_gate"));
   activity=instrument.startActivitySync(new Intent(context,MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
   instrument.runOnMainSync(()->EngineService.command(context,"start"));waitFor(runtime::isRunning,40);
   JSONObject license=runtime.request("/api/engine/v1/license","GET","");assertTrue(license.getString("device_id").startsWith("AVD1-"));assertFalse(license.toString().contains("AMD1-"));report.put("license_status_filtered",true);
   JSONObject doctor=runtime.request("/api/doctor","POST","{}");assertTrue(new JSONObject(doctor.getString("output")).getJSONObject("checks").getBoolean("native_aac_encoded"));report.put("doctor",new JSONObject(doctor.getString("output")));
   JSONObject state=runtime.request("/api/state","GET","");assertEquals("1.1.0",state.getString("app_version"));assertTrue(state.getBoolean("engine_running"));assertTrue(state.getJSONObject("capabilities").getBoolean("test_tone"));report.put("state",state);
   JSONObject offer=runtime.request("/api/plus/offers","GET","");assertEquals("air2mi",offer.getString("app_id"));report.put("offer_fetched",true);
   // CRUD is deliberately disabled: no real receiver or speaker connection is started.
   String speakerID="smoke-"+UUID.randomUUID().toString();String inputID=null;
   try{
    runtime.request("/api/engine/v1/speakers","POST",new JSONObject().put("speaker_id",speakerID).put("display_name","Smoke target").put("ip","127.0.0.1").put("enabled",true).toString());
    if(state.getJSONObject("inputs").getJSONArray("inputs").length()<2){
     JSONObject input=runtime.request("/api/engine/v1/inputs","POST",new JSONObject().put("display_name","Smoke input").put("enabled",true).put("options",new JSONObject().put("speaker_id",speakerID)).toString());
     JSONArray entries=input.optJSONArray("inputs");
     if(entries==null)entries=runtime.request("/api/engine/v1/inputs","GET","").getJSONArray("inputs");
     for(int i=0;i<entries.length();i++)if(entries.getJSONObject(i).getJSONObject("options").optString("speaker_id").equals(speakerID))inputID=entries.getJSONObject(i).getString("input_id");
     assertNotNull(inputID);report.put("automatic_receiver_defaults",true);
    }
   }finally{
    if(inputID!=null)runtime.request("/api/engine/v1/inputs/"+inputID,"DELETE","{}");
    runtime.request("/api/engine/v1/speakers/"+speakerID,"DELETE","{}");
   }
   final Activity view=activity;instrument.runOnMainSync(view::finish);activity=null;
   Thread.sleep(60000);assertTrue("Engine stopped after Activity closed",runtime.isRunning());
   boolean foreground=false;for(ActivityManager.RunningServiceInfo info:context.getSystemService(ActivityManager.class).getRunningServices(100))if(info.service.getClassName().equals(EngineService.class.getName()))foreground=info.foreground;
   assertTrue("OS does not report an actual foreground service",foreground);report.put("activity_closed_60_seconds",true);report.put("foreground_service_confirmed",true);
   NotificationManager notifications=context.getSystemService(NotificationManager.class);PendingIntent stop=null;
   for(android.service.notification.StatusBarNotification notification:notifications.getActiveNotifications())if(notification.getId()==41)stop=notification.getNotification().actions[0].actionIntent;
   assertNotNull(stop);stop.send();waitFor(()->!runtime.isRunning()&&!EngineService.wanted(context),40);report.put("notification_stop",true);runtime.request("/api/doctor","POST","{}");assertFalse(runtime.status().getBoolean("initialized"));assertFalse(runtime.isRunning());report.put("stopped_doctor_cleanup",true);report.put("passed",true);
  }catch(Throwable failure){report.put("passed",false);report.put("error",DeviceDisplay.filter(failure.toString()));throw failure;}
  finally{
   if(activity!=null){Activity view=activity;instrument.runOnMainSync(view::finish);}
   context.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",false).commit();runtime.stopHosted();
   // Do not silently resume a test run after instrumentation exits.
   report.put("previous_desired_running",wasWanted);
   try(FileOutputStream out=new FileOutputStream(new File(context.getFilesDir(),"android-app-test-report.json"))){out.write(DeviceDisplay.filter(report.toString(2)).getBytes(StandardCharsets.UTF_8));}
  }
 }
}
