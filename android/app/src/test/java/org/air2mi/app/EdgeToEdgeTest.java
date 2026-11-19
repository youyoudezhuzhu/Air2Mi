package org.air2mi.app;

import android.app.Activity;
import android.graphics.Insets;
import android.graphics.Rect;
import android.view.DisplayCutout;
import android.view.View;
import android.view.WindowInsets;
import android.widget.FrameLayout;
import java.util.Collections;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.Robolectric;
import org.robolectric.RobolectricTestRunner;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

/** Source added this round; execution deferred to the next authorized build. */
@RunWith(RobolectricTestRunner.class)
public class EdgeToEdgeTest {
    @Test @Config(sdk=34) public void keyboardAndSystemInsetsUseMaximumInsteadOfAdding() {
        Activity activity=Robolectric.buildActivity(Activity.class).setup().get();
        FrameLayout root=EdgeToEdge.wrap(activity,new View(activity));
        WindowInsets insets=new WindowInsets.Builder()
                .setInsets(WindowInsets.Type.statusBars(),Insets.of(0,32,0,0))
                .setInsets(WindowInsets.Type.navigationBars(),Insets.of(0,0,0,24))
                .setInsets(WindowInsets.Type.ime(),Insets.of(0,0,0,360))
                .setVisible(WindowInsets.Type.ime(),true).build();
        assertSame(WindowInsets.CONSUMED,EdgeToEdge.applyInsets(root,insets));
        assertEquals(32,root.getPaddingTop());assertEquals(360,root.getPaddingBottom());
        EdgeToEdge.applyInsets(root,insets);
        assertEquals(32,root.getPaddingTop());assertEquals(360,root.getPaddingBottom());
        EdgeToEdge.applyInsets(root,new WindowInsets.Builder()
                .setInsets(WindowInsets.Type.statusBars(),Insets.of(0,32,0,0))
                .setInsets(WindowInsets.Type.navigationBars(),Insets.of(0,0,0,24)).build());
        assertEquals(24,root.getPaddingBottom());
    }
    @Test @Config(sdk=29) public void legacyCutoutProtectsSideContent() {
        Activity activity=Robolectric.buildActivity(Activity.class).setup().get();
        FrameLayout root=EdgeToEdge.wrap(activity,new View(activity));
        DisplayCutout cutout=new DisplayCutout(new Rect(36,0,0,0),
                Collections.singletonList(new Rect(0,0,36,120)));
        WindowInsets insets=new WindowInsets.Builder().setSystemWindowInsets(Insets.of(0,24,0,24))
                .setDisplayCutout(cutout).build();
        EdgeToEdge.applyInsets(root,insets);
        assertEquals(36,root.getPaddingLeft());assertEquals(24,root.getPaddingTop());
        assertEquals(24,root.getPaddingBottom());
        assertTrue((activity.getWindow().getDecorView().getSystemUiVisibility()
                & View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR)!=0);
    }
}
