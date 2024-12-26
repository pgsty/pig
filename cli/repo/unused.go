package repo

// // AddPigstyRpmRepo adds the Pigsty RPM repository to the system
// func AddPigstyRpmRepo(region string) error {
// 	if err := RpmPrecheck(); err != nil {
// 		return err
// 	}
// 	LoadRpmRepo(embedRpmRepo)
// 	if region == "" { // check network condition (if region is not set)
// 		get.Timeout = time.Second
// 		get.NetworkCondition()
// 		if !get.InternetAccess {
// 			logrus.Warn("no internet access, assume region = default")
// 			region = "default"
// 		}
// 	}

// 	// write gpg key
// 	err := TryReadMkdirWrite(pigstyRpmGPGPath, embedGPGKey)
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("import gpg key B9BD8B20 to %s", pigstyRpmGPGPath)

// 	// write repo file
// 	repoContent := ModuleRepoConfig("pigsty", region)
// 	err = TryReadMkdirWrite(ModuleRepoPath("pigsty"), []byte(repoContent))
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("repo added: %s", ModuleRepoPath("pigsty"))
// 	return nil
// }

// // RemovePigstyRpmRepo removes the Pigsty RPM repository from the system
// func RemovePigstyRpmRepo() error {
// 	if err := RpmPrecheck(); err != nil {
// 		return err
// 	}

// 	// wipe pigsty repo file
// 	err := WipeFile(ModuleRepoPath("pigsty"))
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("remove %s", ModuleRepoPath("pigsty"))

// 	// wipe pigsty gpg file
// 	err = WipeFile(pigstyRpmGPGPath)
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("remove gpg key B9BD8B20 from %s", pigstyRpmGPGPath)
// 	return nil
// }
