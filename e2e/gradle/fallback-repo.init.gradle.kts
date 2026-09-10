// JitPack no longer serves com.github.arkon.FlexibleAdapter:flexible-adapter:c8013533 (its
// rebuild fails on retired jcenter plugins), so the copies under e2e/gradle/m2 are offered as
// a last-resort repository. Gradle only consults it after the project's own repositories.
val fallback = File(System.getenv("SYNCYOMI_M2_FALLBACK") ?: error("SYNCYOMI_M2_FALLBACK is not set")).toURI()

settingsEvaluated {
    dependencyResolutionManagement.repositories.maven { url = fallback }
}
