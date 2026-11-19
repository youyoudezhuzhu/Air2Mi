package org.air2mi.app;

import android.app.Activity;
import android.graphics.Color;
import android.graphics.Insets;
import android.graphics.drawable.ColorDrawable;
import android.os.Build;
import android.view.DisplayCutout;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.widget.FrameLayout;

/** Transparent system bars with a single native owner for content-safe insets. */
final class EdgeToEdge {
    static final int SURFACE = Color.rgb(247, 249, 252); // Fluent web background #f7f9fc.

    static FrameLayout wrap(Activity activity, View content) {
        Window window = activity.getWindow();
        window.setBackgroundDrawable(new ColorDrawable(SURFACE));
        window.clearFlags(WindowManager.LayoutParams.FLAG_TRANSLUCENT_STATUS
                | WindowManager.LayoutParams.FLAG_TRANSLUCENT_NAVIGATION);
        window.addFlags(WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS);
        window.setStatusBarColor(Color.TRANSPARENT);
        window.setNavigationBarColor(Color.TRANSPARENT);
        window.setNavigationBarDividerColor(Color.TRANSPARENT);
        window.setStatusBarContrastEnforced(false);
        window.setNavigationBarContrastEnforced(false);
        WindowManager.LayoutParams attributes = window.getAttributes();
        attributes.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
        window.setAttributes(attributes);
        if (Build.VERSION.SDK_INT >= 30) {
            window.setDecorFitsSystemWindows(false);
            WindowInsetsController controller = window.getInsetsController();
            if (controller != null) {
                int appearance = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                        | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                controller.setSystemBarsAppearance(appearance, appearance);
            }
        } else {
            // API 29 is the minimum supported Android version.
            window.getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                    | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                    | View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR);
        }
        FrameLayout surface = new FrameLayout(activity);
        surface.setBackgroundColor(SURFACE);
        content.setBackgroundColor(SURFACE);
        surface.addView(content, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        surface.setOnApplyWindowInsetsListener((view, insets) -> applyInsets(view, insets));
        surface.addOnAttachStateChangeListener(new View.OnAttachStateChangeListener() {
            @Override public void onViewAttachedToWindow(View view) { view.requestApplyInsets(); }
            @Override public void onViewDetachedFromWindow(View view) { }
        });
        return surface;
    }

    static WindowInsets applyInsets(View surface, WindowInsets insets) {
        if (Build.VERSION.SDK_INT >= 30) {
            Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
            Insets keyboard = insets.getInsets(WindowInsets.Type.ime());
            surface.setPadding(Math.max(bars.left, keyboard.left), Math.max(bars.top, keyboard.top),
                    Math.max(bars.right, keyboard.right), Math.max(bars.bottom, keyboard.bottom));
            // The WebView already occupies the inset child bounds: do not apply these twice.
            return WindowInsets.CONSUMED;
        }
        DisplayCutout cutout = insets.getDisplayCutout();
        surface.setPadding(Math.max(insets.getSystemWindowInsetLeft(), cutout == null ? 0 : cutout.getSafeInsetLeft()),
                Math.max(insets.getSystemWindowInsetTop(), cutout == null ? 0 : cutout.getSafeInsetTop()),
                Math.max(insets.getSystemWindowInsetRight(), cutout == null ? 0 : cutout.getSafeInsetRight()),
                Math.max(insets.getSystemWindowInsetBottom(), cutout == null ? 0 : cutout.getSafeInsetBottom()));
        // On API 29 adjustResize provides the keyboard viewport resize.
        return insets.consumeSystemWindowInsets().consumeDisplayCutout();
    }
}
