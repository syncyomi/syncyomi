val fallback = File(System.getenv("SYNCYOMI_M2_FALLBACK") ?: error("SYNCYOMI_M2_FALLBACK is not set")).toURI()

settingsEvaluated {
    dependencyResolutionManagement.repositories.maven { url = fallback }
}
