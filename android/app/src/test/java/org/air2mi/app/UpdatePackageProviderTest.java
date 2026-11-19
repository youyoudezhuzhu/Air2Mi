package org.air2mi.app;
import static org.junit.Assert.*;
import android.net.Uri;
import android.content.Context;
import android.os.ParcelFileDescriptor;
import org.robolectric.RuntimeEnvironment;
import org.robolectric.annotation.Config;
import java.io.*;
import java.nio.file.Files;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.RobolectricTestRunner;
import org.robolectric.Robolectric;
@RunWith(RobolectricTestRunner.class) @Config(sdk=34)
public class UpdatePackageProviderTest {
 @Test public void grantsOnlyTwoFixedReadOnlyPackages()throws Exception{
  Context context=RuntimeEnvironment.getApplication();
  UpdatePackageProvider provider=Robolectric.buildContentProvider(UpdatePackageProvider.class).create("org.air2mi.app.engine-update").get();
  for(String part:new String[]{"app","engine"}){
   File file=new File(context.getFilesDir(),"online-updates-"+part+"/"+part+"-update.apk");file.getParentFile().mkdirs();Files.write(file.toPath(),new byte[]{1,2,3});
   Uri uri=Uri.parse("content://org.air2mi.app.engine-update/"+part+"-update.apk");try(ParcelFileDescriptor opened=provider.openFile(uri,"r")){assertNotNull(opened);}
   try{provider.openFile(uri,"w");fail("write granted");}catch(FileNotFoundException expected){}
  }
  for(String path:new String[]{"/../engine-data/identity","/engine-update.apk?path=identity","/app-update.apk#private","/arbitrary.apk"}){try{provider.openFile(Uri.parse("content://org.air2mi.app.engine-update"+path),"r");fail("untrusted path granted");}catch(FileNotFoundException expected){}}
 }
}
