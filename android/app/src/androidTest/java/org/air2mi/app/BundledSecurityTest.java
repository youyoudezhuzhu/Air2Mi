package org.air2mi.app;
import android.content.Context;
import android.os.Bundle;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import com.chaquo.python.Python;
import com.chaquo.python.PyObject;
import java.nio.charset.StandardCharsets;
import org.json.JSONObject;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.*;
import static org.junit.Assume.*;
@RunWith(AndroidJUnit4.class)
public class BundledSecurityTest {
 @Test public void actualNativeGateAndTamperResistance()throws Exception{
  assumeTrue(BuildConfig.BUNDLED_TEST_ENGINE);
  Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();
  EngineRuntime runtime=EngineRuntime.get(context);
  try{
   runtime.ensureBundledEngine();
   assertEquals("0.3.11-stable.1-hardened",runtime.installedManifest().getString("package_version"));
   runtime.initialize();
   String script;
   try(java.io.InputStream input=InstrumentationRegistry.getInstrumentation().getContext().getAssets().open("security_probe.py")){script=new String(input.readAllBytes(),StandardCharsets.UTF_8);}
   PyObject scope=Python.getInstance().getModule("builtins").callAttr("dict");
   Python.getInstance().getModule("builtins").callAttr("exec",script,scope);
   String result=Python.getInstance().getModule("json").callAttr("dumps",scope.callAttr("__getitem__","run").call()).toString();
   JSONObject report=new JSONObject(result);
   assertTrue(report.getBoolean("crypto_and_parser_replacement_rejected"));
   runtime.startHosted();
   assertTrue(runtime.isRunning());
   JSONObject doctor=runtime.request("/api/doctor","POST","{}");
   assertTrue(new JSONObject(doctor.getString("output")).getJSONObject("checks").getBoolean("native_aac_encoded"));
   report.put("actual_api_and_aac",true);
   Bundle status=new Bundle();status.putString("security_report",report.toString());InstrumentationRegistry.getInstrumentation().sendStatus(0,status);
  }finally{runtime.stopHosted();}
 }
}
