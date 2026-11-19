package org.air2mi.app;
import android.content.Context;
import android.os.Bundle;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import com.chaquo.python.*;
import java.io.File;
import org.json.JSONObject;
import org.junit.Test;
import org.junit.runner.RunWith;
import static org.junit.Assert.*;
import static org.junit.Assume.*;
/** Explicit device network check. Never requests OS installation or a real CDK. */
@RunWith(AndroidJUnit4.class)
public class OnlineUpdateNetworkTest {
 @Test public void independentSignedCatalogsOverRealAndroidTLS()throws Exception{
  assumeTrue(BuildConfig.BUNDLED_TEST_ENGINE);
  Context context=InstrumentationRegistry.getInstrumentation().getTargetContext();
  EngineRuntime runtime=EngineRuntime.get(context);runtime.ensureBundledEngine();
  // Load the exact application transport, then use a previous-version fixture
  // with the formal package ID. The isolated test App cannot install that APK.
  runtime.onlineUpdates("app");
  String key;try(java.io.InputStream in=context.getAssets().open("engine-update-trust.pub")){key=new String(in.readAllBytes(),java.nio.charset.StandardCharsets.UTF_8).trim();}
  PyObject factory=Python.getInstance().getModule("air2mi_update_client").get("Client");
  PyObject app=factory.call(new File(context.getCacheDir(),"network-app").getAbsolutePath(),key,"org.air2mi.app",0,null,false,"app","0.0.0");
  PyObject engine=factory.call(new File(context.getCacheDir(),"network-engine").getAbsolutePath(),key,"org.air2mi.app",0,null,false,"engine","0.0.0");
  JSONObject a=new JSONObject(app.callAttr("check").toString());
  JSONObject e=new JSONObject(engine.callAttr("check","{\"package_version\":\"0.0.0\"}").toString());
  assertTrue(a.toString(),a.getBoolean("available"));assertTrue(e.toString(),e.getBoolean("available"));
  assertEquals("app",a.getString("component"));assertEquals("engine",e.getString("component"));
  assertFalse(a.getString("app_version").isEmpty());assertFalse(e.getString("package_version").isEmpty());
  Bundle result=new Bundle();result.putString("network_update_report",new JSONObject().put("android_tls",true).put("anonymous_pages",true).put("signed_app_catalog",true).put("signed_engine_catalog",true).put("app_version",a.getString("app_version")).put("engine_package_version",e.getString("package_version")).put("os_install_requested",false).toString());
  InstrumentationRegistry.getInstrumentation().sendStatus(0,result);
 }
}
